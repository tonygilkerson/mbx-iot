package main

import (
	"fmt"
	"log"
	"machine"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/tonygilkerson/mbx-iot/internal/dsp"
	"github.com/tonygilkerson/mbx-iot/internal/road"
	"github.com/tonygilkerson/mbx-iot/internal/umsg"
	"github.com/tonygilkerson/mbx-iot/pkg/iot"
	"tinygo.org/x/drivers/sx127x"
)

const (
	HEARTBEAT_DURATION_SECONDS = 10
)

/////////////////////////////////////////////////////////////////////////////
//			Main
/////////////////////////////////////////////////////////////////////////////

func main() {

	//
	// Named PINs
	//
	var en machine.Pin = machine.GP15
	var sdi machine.Pin = machine.GP16 // machine.SPI0_SDI_PIN
	var cs machine.Pin = machine.GP17
	var sck machine.Pin = machine.GP18 // machine.SPI0_SCK_PIN
	var sdo machine.Pin = machine.GP19 // machine.SPI0_SDO_PIN
	var rst machine.Pin = machine.GP20
	var dio0 machine.Pin = machine.GP21  // (GP21--G0) Must be connected from pico to breakout for radio events IRQ to work
	var dio1 machine.Pin = machine.GP22  // (GP22--G1)I don't now what this does but it seems to need to be connected
	var uartTx machine.Pin = machine.GP0 // machine.UART0_TX_PIN
	var uartRx machine.Pin = machine.GP1 // machine.UART0_RX_PIN
	var led machine.Pin = machine.GPIO25 // GP25 machine.LED

	//
	// run light
	//
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})
	dsp.RunLight(led, 10)

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	//
	// setup Uart
	//
	log.Println("Configure UART")
	uart := machine.UART0
	uart.Configure(machine.UARTConfig{BaudRate: 115200, TX: uartTx, RX: uartRx})

	//
	// 	Setup Lora
	//
	var loraRadio *sx127x.Device
	// I am thinking that a batch of message can be half dozen max so 250 should be plenty large
	txQ := make(chan string, 250)
	rxQ := make(chan string, 250)

	log.Println("Setup LORA")
	radio := road.SetupLora(
		*machine.SPI0, 
		en, 
		rst, 
		cs, 
		dio0, 
		dio1, 
		sck, 
		sdo, 
		sdi, 
		loraRadio, 
		&txQ, 
		&rxQ, 
		5_000, 
		10_000, 
		1, 
		road.TxRx)

	// Create status map
	statusMap := make(map[string]string)

	// Launch go routines
	log.Println("Launch go routines")
	go writeToSerial(&rxQ, uart, statusMap)
	go readFromSerial(&txQ, uart)
	go radio.LoraRxTxRunner()

	// Main loop
	log.Println("Start main loop")

	ticker := time.NewTicker(time.Second * HEARTBEAT_DURATION_SECONDS)
	var count int

	for range ticker.C {

		log.Printf("------------------mbx-iot gateway MainLoopHeartbeat-------------------- %v", count)
		count += 1
		statusMap[iot.GatewayHeartbeat] = strconv.Itoa(count)

		// Send out status on each heartbeat
		publishStatus(statusMap, txQ)

		dsp.RunLight(led, 2)
		runtime.Gosched()
	}

}

///////////////////////////////////////////////////////////////////////////////
//
//	Functions
//
///////////////////////////////////////////////////////////////////////////////

func publishStatus(statusMap map[string]string, txQ chan string) {

	// DEVTODO - Add a filter and only publish certain status
	for k, v := range statusMap {
		txQ <- fmt.Sprintf("%s:%s", k, v)
	}
	
}

func writeToSerial(rxQ *chan string, uart *machine.UART, statusMap map[string]string) {
	var msgBatch string
	var count int

	for msgBatch = range *rxQ {
		count += 1
		log.Printf("gateway.writeToSerial: Message batch: [%v]", msgBatch)

		//
		// Split the batch of message into individual message and save the 
		// status for the ones we are interested in
		//
		messages := road.SplitMessageBatch(msgBatch)
		for _, msg := range messages {
			log.Printf("gateway.writeToSerial: Write to serial: [%v]", msg)
			uart.Write(append([]byte(msg), umsg.TOKEN_PIPE))

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

			switch  {
			case msgKey == iot.MbxDoorOpened:
				c,_ := strconv.Atoi(statusMap[iot.MbxDoorOpened])
				c += 1
				log.Printf("gateway.writeToSerial: increment MbxDoorOpened count to [%v]", c)
				statusMap[msgKey] = strconv.Itoa(c)

			case msgKey == iot.MbxMuleAlarm:
				c,_ := strconv.Atoi(statusMap[iot.MbxMuleAlarm])
				c += 1
				log.Printf("gateway.writeToSerial: increment MbxMuleAlarm count to [%v]", c)
				statusMap[msgKey] = strconv.Itoa(c)
			
			case msgKey == iot.MbxTemperature:
				log.Printf("gateway.writeToSerial: set MbxTemperature status to [%v]", msgValue)
				statusMap[msgKey] = msgValue
			}
			
		}

		runtime.Gosched()

	}

}

//
// readFromSerial will read messages sent from the cluster and broadcast them for receive in the field
//                currently this is used for testing. The cluster exposed a REST endpoint that can be
//                post a message that is subsequently read and then transmitted here
//
func readFromSerial(txQ *chan string, uart *machine.UART) {
	data := make([]byte, 250)

	ticker := time.NewTicker(time.Second * 1)
	for range ticker.C {

		//
		// Check to see if we have any data to read
		//
		if uart.Buffered() == 0 {
			//Serial buffer is empty, nothing to do, get out!"
			continue
		}

		//
		// Read from serial then transmit the message
		//
		n, err := uart.Read(data)
		if err != nil {
			log.Printf("Serial read error [%v]", err)
		} else {
			log.Printf("Put on txQ [%v]", string(data[:n]))
			*txQ <- string(data[:n])
		}

		runtime.Gosched()
	}

}

