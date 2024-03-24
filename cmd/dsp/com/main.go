// The program is the communication component for the display
//
// It manages the LORA RX/TX cycle.  Messages received via LORA are
// send to the epaper component via the UART message bus
package main

import (
	"log"
	"machine"
	"runtime"
	"strings"
	"time"

	"github.com/tonygilkerson/mbx-iot/internal/dsp"
	"github.com/tonygilkerson/mbx-iot/internal/road"
	"github.com/tonygilkerson/mbx-iot/internal/umsg"
	"github.com/tonygilkerson/mbx-iot/pkg/iot"
	"tinygo.org/x/drivers/sx127x"
)

const (
	SENDER_ID                  = "dsp.com"
	HEARTBEAT_DURATION_SECONDS = 15

	// DEVTODO - I want this to be large
	TXRX_LOOP_TICKER_DURATION_SECONDS = 10
)

/////////////////////////////////////////////////////////////////////////////
//			Main
/////////////////////////////////////////////////////////////////////////////

func main() {

	//
	// Named PINs
	//

	var uartInTx machine.Pin = machine.GP0  // UART0
	var uartInRx machine.Pin = machine.GP1  // UART0
	var uartOutTx machine.Pin = machine.GP4 // UART1
	var uartOutRx machine.Pin = machine.GP5 // UART1

	var loraEn machine.Pin = machine.GP15
	var loraSdi machine.Pin = machine.GP16 // machine.SPI0_SDI_PIN
	var loraCs machine.Pin = machine.GP17
	var loraSck machine.Pin = machine.GP18 // machine.SPI0_SCK_PIN
	var loraSdo machine.Pin = machine.GP19 // machine.SPI0_SDO_PIN
	var loraRst machine.Pin = machine.GP20
	var loraDio0 machine.Pin = machine.GP21 // (GP21--G0) Must be connected from pico to breakout for radio events IRQ to work
	var loraDio1 machine.Pin = machine.GP22 // (GP22--G1) I don't now what this does but it seems to need to be connected

	var led machine.Pin = machine.GPIO25 // GP25 machine.LED

	//
	// UARTs
	//
	uartIn := machine.UART0
	uartOut := machine.UART1

	//
	// run light
	//
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})
	dsp.RunLight(led, 10)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	/////////////////////////////////////////////////////////////////////////////
	// Broker
	/////////////////////////////////////////////////////////////////////////////

	fooCh := make(chan umsg.FooMsg)
	statusCh := make(chan umsg.StatusMsg)

	mb := umsg.NewBroker(
		SENDER_ID,

		uartIn,
		uartInTx,
		uartInRx,

		uartOut,
		uartOutTx,
		uartOutRx,

		fooCh,
		statusCh,
	)
	log.Printf("[main] - configure message broker\n")
	mb.Configure()

	//
	// 	Setup Lora
	//
	var loraRadio *sx127x.Device
	txQ := make(chan string, 250) // I would hope the channel size would never be larger than ~4 so 250 is large
	rxQ := make(chan string, 250)

	log.Println("Setup LORA")
	radio := road.SetupLora(*machine.SPI0, loraEn, loraRst, loraCs, loraDio0, loraDio1, loraSck, loraSdo, loraSdi, loraRadio, &txQ, &rxQ, 0, 10_000, TXRX_LOOP_TICKER_DURATION_SECONDS, road.TxRx)

	// Routine to send and receive
	go radio.LoraRxTxRunner()

	//
	// Main loop
	//
	ticker := time.NewTicker(time.Second * HEARTBEAT_DURATION_SECONDS)
	var count int

	for range ticker.C {

		log.Printf("------------------DspMainLoopHeartbeat-------------------- %v", count)
		count += 1

		//
		// Send Heartbeat to Tx queue
		//
		txQ <- iot.DspMainLoopHeartbeat
		dsp.RunLight(led, 2)

		//
		// Consume any messages received
		//
		rxQConsumer(&rxQ, &mb)

		//
		// Let someone else have a turn
		//
		runtime.Gosched()
	}

}

///////////////////////////////////////////////////////////////////////////////
//
//	Functions
//
///////////////////////////////////////////////////////////////////////////////

// DEVTODO - I don't think I need a channel pointer here when I have all
//
//	working I should change this to see if it still works
func rxQConsumer(rxQ *chan string, mb *umsg.MsgBroker) {
	var msgBatch string

	for len(*rxQ) > 0 {

		//A batch look like: "msg1|msg2|msg3|..."
		msgBatch = <-*rxQ
		log.Printf("dsp.com.rxQConsumer: Message batch: [%v]", msgBatch)

		messages := road.SplitMessageBatch(msgBatch)
		for _, msg := range messages {
			log.Printf("dsp.com.rxQConsumer: Message: [%v]", msg)

			//
			// Each message is a key:values pair
			//
			parts := strings.Split(msg, ":")
			var msgKey string
			var msgValue string

			if len(parts) > 0 {
				msgKey = parts[0]
			}
			if len(parts) > 1 {
				msgValue = parts[1]
			}

			//
			// Send stats to display over UART
			//
			// if msgKey == string(umsg.MSG_STATUS) {  DEVTODO - what up with this?
			if msgKey == string(iot.GatewayHeartbeat) || msgKey == string(iot.MbxDoorOpened) {
				publishStatusToUart(mb, msgKey, msgValue)
			}

			// Insert a small pause here to give the consumer a change to read the message
			// this in an effort to not fill up the buffer and lose data
			runtime.Gosched()
			time.Sleep(time.Millisecond * 100)

		}

	}
}

// Send Status
func publishStatusToUart(mb *umsg.MsgBroker, msgKey string, msgValue string) {

	var statusMsg umsg.StatusMsg
	statusMsg.Kind = umsg.MSG_STATUS
	statusMsg.SenderID = SENDER_ID
	statusMsg.Key = msgKey
	statusMsg.Value = msgValue

	log.Printf("dsp.com.sendStatus: Publish on message bus, Status key: %s, value: %s", msgKey, msgValue)
	mb.PublishStatusToUart(statusMsg)

}
