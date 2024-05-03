package main

import (
	"log"
	"machine"
	"math"
	"runtime"
	"time"

	"github.com/tonygilkerson/mbx-iot/internal/dsp"
	"github.com/tonygilkerson/mbx-iot/internal/umsg"
	"github.com/tonygilkerson/mbx-iot/pkg/iot"
	"tinygo.org/x/drivers/waveshare-epd/epd4in2"
	"tinygo.org/x/drivers/tone"
)

const (
	SENDER_ID = "dsp.epaper"
	HEARTBEAT_DURATION_SECONDS = 60
	HEARTBEAT_MOD = 15 // Update screen every 15 min to update age
)

var display epd4in2.Device

func main() {

	//
	// Named PINs
	//
	var uartInTx machine.Pin = machine.GP0 // UART0
	var uartInRx machine.Pin = machine.GP1 // UART0

	var mbxDoorOpenedAckBtn machine.Pin = machine.GP2 // Acknowledges the fact that we got mail, make alerts turn off
	var requestBtn machine.Pin = machine.GP3          // System request, will cycle the heart beat loop and refresh status

	var uartOutTx machine.Pin = machine.GP4 // UART1
	var uartOutRx machine.Pin = machine.GP5 // UART1
	var neo machine.Pin = machine.GP6       // Neopixel DIN
	var buzzerPin machine.Pin = machine.GP7 // Buzzer DIN

	var dc machine.Pin = machine.GP11   // pin15
	var rst machine.Pin = machine.GP12  // pin16
	var busy machine.Pin = machine.GP13 // pin17
	var cs machine.Pin = machine.GP17   // pin22
	var clk machine.Pin = machine.GP18  // pin24 machine.SPI0_SCK_PIN
	var din machine.Pin = machine.GP19  // pin25 machine.SPI0_SDO_PIN

	var led machine.Pin = machine.GPIO25 // GP25 machine.LED

	//
	// run light
	//
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})
	dsp.RunLight(led, 10)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	//
	// PWM for tone alarm
	//
	buzzer, err := tone.New(machine.PWM3, buzzerPin)
	if err != nil {
		log.Panicln("failed to configure PWM")
	}
	soundSiren(buzzer)


	//
	// Neo Pixel
	//
	neo.Configure(machine.PinConfig{Mode: machine.PinOutput})

	//
	// Buttons
	//
	mbxDoorOpenedAckBtnCh := make(chan string, 1)
	mbxDoorOpenedAckBtn.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	mbxDoorOpenedAckBtn.SetInterrupt(machine.PinRising, func(p machine.Pin) {
		// Use non-blocking send so if the channel buffer is full,
		// the value will get dropped instead of crashing the system
		select {
		case mbxDoorOpenedAckBtnCh <- "rise":
		default:
		}

	})

	requestBtnCh := make(chan string, 1)
	requestBtn.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	requestBtn.SetInterrupt(machine.PinRising, func(p machine.Pin) {
		// Use non-blocking send so if the channel buffer is full,
		// the value will get dropped instead of crashing the system
		select {
		case requestBtnCh <- "rise":
		default:
		}

	})

	//
	// UARTs
	//
	uartIn := machine.UART0
	uartOut := machine.UART1



	/////////////////////////////////////////////////////////////////////////////
	// Broker
	/////////////////////////////////////////////////////////////////////////////

	fooCh := make(chan umsg.FooMsg)
	statusCh := make(chan umsg.StatusMsg, 5)

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
	log.Printf("dsp.epaper.main: configure message broker\n")
	mb.Configure()

	//
	// SPI
	//
	log.Println("dsp.epaper.main: Configure SPI...")
	machine.SPI0.Configure(machine.SPIConfig{
		Frequency: 8000000,
		Mode:      0,
		SCK:       clk,
		SDO:       din,
		// SDI:       sdi,
	})

	//
	// Display
	//
	log.Println("dsp.epaper.main: new epd4in2")
	display = epd4in2.New(machine.SPI0, cs, dc, rst, busy)
	display.Configure(epd4in2.Config{})
	content := dsp.NewContent()

	//
	//  Main loop
	//

	// Non-blocking ch read that will timeout... boom!
	boom := time.NewTicker(time.Second * HEARTBEAT_DURATION_SECONDS)
	var displayNeedsRefreshed bool = true
	var isDirtyCount int
	var count float64

	for {
		// Refresh age each cycle
		content.UpdateAge()
		count += 1

		// Wait for button or timeout
		log.Printf("dsp.epaper.main: wait on a button to be pushed or a timeout, IsDirty: %v", content.IsDirty() )

		select {

		case <-requestBtnCh:
			log.Println("dsp.epaper.main: requestBtn Hit!!!!")
			dsp.NeoBlink(neo)
			displayNeedsRefreshed = true

		case <-mbxDoorOpenedAckBtnCh:
			log.Println("dsp.epaper.main: mbxDoorOpenedAckBtn Hit!!!!")
			content.SetIsDirty(false)
			content.SetYouGotMailIndicator("waiting...")
			displayNeedsRefreshed = true

		case <-boom.C:
			log.Printf("dsp.epaper.main:  Boom! heartbeat timeout\n")
			displayNeedsRefreshed = false

		}

		//
		// Check to see if content needs updated
		//
		log.Println("dsp.epaper.main: Read all messages on the buffer")
		mb.UartReader()
		consumeAllStatusFromChToUpdateContent(statusCh, content)

		//
		// Is the content dirty?
		//
		if content.IsDirty() {
			// Get someone's attention
			log.Println("dsp.epaper.main: Nightrider")
			dsp.NeoNightrider(neo)
			soundSiren(buzzer)
			isDirtyCount += 1
		} else {
			isDirtyCount = 0
		}

		// Refresh the display the first time
		if isDirtyCount == 1 {
			log.Println("dsp.epaper.main: First isDirty")
			displayNeedsRefreshed = true
		}

		if math.Mod(count, HEARTBEAT_MOD) == 0 {
			displayNeedsRefreshed = true
		}
		
		//
		// Display Content
		//
		if displayNeedsRefreshed {
			log.Println("dsp.epaper.main: DisplayContent()")
			dsp.ClearDisplay(&display)
			content.DisplayContent(&display)
		}

		log.Println("dsp.epaper.main: Gosched()")
		runtime.Gosched()
	}

}

///////////////////////////////////////////////////////////////////////////////
//
//	Functions
//
///////////////////////////////////////////////////////////////////////////////

// consumeAllStatusFromChToUpdateContent
func consumeAllStatusFromChToUpdateContent(statusCh chan umsg.StatusMsg, content *dsp.Content) {

	var msg umsg.StatusMsg

	//
	// Receive all status messages
	//
	for len(statusCh) > 0 {

		msg = <-statusCh
		log.Printf("dsp.epaper.consumeAllStatusFromChToUpdateContent: msg: [%v]\n", msg)

		//
		// Update display content depending on the type of status received
		//DEVTODO make this more general
		//        maybe this should be in a different function
		switch msg.Key {
		case iot.GatewayHeartbeat:
			log.Printf("dsp.epaper.consumeAllStatusFromChToUpdateContent: call SetGatewayHeartbeat()")
			content.SetGatewayHeartbeatStatus(msg.Value)
		case iot.MbxDoorOpened:
			log.Printf("dsp.epaper.consumeAllStatusFromChToUpdateContent: call SetMbxDoorOpened()")
			content.SetMbxDoorOpenedStatus(msg.Value)
		default:
			log.Printf("dsp.epaper.consumeAllStatusFromChToUpdateContent: Not interested in this content: %v", msg)
		}
	}
}

func soundSiren(buzzer tone.Speaker) {
	for i := 0; i < 10; i++ {
		log.Println("nee")
		buzzer.SetNote(tone.B5)
		time.Sleep(time.Second / 2)

		log.Println("naw")
		buzzer.SetNote(tone.A5)
		time.Sleep(time.Second / 2)

	}
	buzzer.Stop()
}
