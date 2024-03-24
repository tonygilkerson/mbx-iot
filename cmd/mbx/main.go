package main

import (
	"fmt"
	"log"
	"machine"
	"runtime"
	"time"

	"tinygo.org/x/drivers/sx127x"
	"github.com/tonygilkerson/mbx-iot/internal/dsp"
	"github.com/tonygilkerson/mbx-iot/internal/road"
	"github.com/tonygilkerson/mbx-iot/pkg/iot"
)

const (
	HEARTBEAT_DURATION_SECONDS = 300
)


/////////////////////////////////////////////////////////////////////////////
//			Main
/////////////////////////////////////////////////////////////////////////////

func main() {

	//
	// Named PINs
	//
	var chg machine.Pin = machine.GP10
	var pgood machine.Pin = machine.GP11
	var mulePin machine.Pin = machine.GP12
	var mailPin machine.Pin = machine.GP13
	var en machine.Pin = machine.GP15
	var sdi machine.Pin = machine.GP16 // machine.SPI0_SDI_PIN
	var cs machine.Pin = machine.GP17
	var sck machine.Pin = machine.GP18 // machine.SPI0_SCK_PIN
	var sdo machine.Pin = machine.GP19 // machine.SPI0_SDO_PIN
	var rst machine.Pin = machine.GP20
	var dio0 machine.Pin = machine.GP21 // (GP21--G0) Must be connected from pico to breakout for radio events IRQ to work
	var dio1 machine.Pin = machine.GP22 // (GP22--G1)I don't now what this does but it seems to need to be connected
	var led machine.Pin = machine.GPIO25 // GP25 machine.LED

	//
	// run light
	//
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})
	dsp.RunLight(led, 10)

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	//
	// 	Setup Lora
	//
	var loraRadio *sx127x.Device
	txQ := make(chan string, 250) // I would hope the channel size would never be larger than ~4 so 250 is large
	rxQ := make(chan string) // this app currently does not do anything with messages received

	radio := road.SetupLora(*machine.SPI0, en, rst, cs, dio0, dio1, sck, sdo, sdi, loraRadio, &txQ, &rxQ, 5_000, 10_000, 10, road.TxOnly)

	//
	// Setup charger
	//

	// CHG - Charge status (active low) pulls to GND (open drain) lighting the connected led when the battery is charging.
	// If the battery is charged or the charger is disabled, CHG is disconnected from ground (high impedance) and the LED will be off.
	chg.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	log.Printf("chg status: %v\n", chg.Get())

	// PGOOD - Power Good Status (active low). PGOOD pulls to GND (open drain) lighting the connected led when a valid input source is connected.
	// If the input power source is not within specified limits, PGOOD is disconnected from ground (high impedance) and the LED will be off.
	pgood.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	log.Printf("pgood status: %v\n", pgood.Get())

	//
	// Setup Mule
	//
	muleInterruptEvents := make(chan string)
	mulePin.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	log.Printf("mulePin status: %v\n", mulePin.Get())

	mulePin.SetInterrupt(machine.PinRising, func(p machine.Pin) {

		// Use non-blocking send so if the channel buffer is full,
		// the value will get dropped instead of crashing the system
		select {
		case muleInterruptEvents <- "up":
		default:
		}

	})

	//
	// Setup mail
	//
	mailInterruptEvents := make(chan string)
	mailPin.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	log.Printf("mailPin status: %v\n", mulePin.Get())

	mailPin.SetInterrupt(machine.PinRising, func(p machine.Pin) {

		// Use non-blocking send so if the channel buffer is full,
		// the value will get dropped instead of crashing the system
		select {
		case mailInterruptEvents <- "up":
		default:
		}

	})

	// Launch go routines

	go mailMonitor(&mailInterruptEvents, &txQ)
	go muleMonitor(&muleInterruptEvents, &txQ)
	go radio.LoraRxTxRunner()

	// Main loop
	ticker := time.NewTicker(time.Second * HEARTBEAT_DURATION_SECONDS)
	var count int
	
	for range ticker.C {
		
		log.Printf("------------------MainLoopHeartbeat-------------------- %v", count)
		count += 1
		log.Printf("mailPin status: %v\n", mailPin.Get())
		log.Printf("mulePin status: %v\n", mulePin.Get())

		//
		// Send Heartbeat to Tx queue
		//
		txQ <- iot.MbxRoadMainLoopHeartbeat

		//
		// send charger status
		//
		sendChargerStatus(chg, pgood, &txQ)

		//
		// Send Temperature to Tx queue
		//
		sendTemperature(&txQ)

		runtime.Gosched()
	}

}

///////////////////////////////////////////////////////////////////////////////
//
//	Functions
//
///////////////////////////////////////////////////////////////////////////////

func mailMonitor(ch *chan string, txQ *chan string) {

	for range *ch {
		log.Println("Mailbox light up")
		*txQ <- iot.MbxDoorOpened

		runtime.Gosched()
		// Wait a long time to give mail man time to shut the door
		time.Sleep(time.Second * 60)
		log.Println("Mailbox light down")

	}

}

func muleMonitor(ch *chan string, txQ *chan string) {

	for range *ch {
		log.Println("Mule light up")
		*txQ <- iot.MbxMuleAlarm

		runtime.Gosched()
		time.Sleep(time.Second * 4)
		log.Println("Mule light down")

	}
}

func sendTemperature(txQ *chan string) {

	// F = ( (ReadTemperature /1000) * 9/5) + 32
	fahrenheit := ((machine.ReadTemperature() / 1000) * 9 / 5) + 32
	fmt.Printf("fahrenheit: %v\n", fahrenheit)
	*txQ <- fmt.Sprintf("%v:%v",iot.MbxTemperature,fahrenheit)

}

func sendChargerStatus(chgPin machine.Pin, pgoodPin machine.Pin, txQ *chan string) {

	if pgoodPin.Get() {
		log.Println("Power source bad")
		*txQ <- iot.MbxChargerPowerSourceBad
	} else {
		log.Println("Power source good")
		*txQ <- iot.MbxChargerPowerSourceGood
	}

	if chgPin.Get() {
		log.Println("Charger off")
		*txQ <- iot.MbxChargerChargeStatusOff
	} else {
		log.Println("Charger on")
		*txQ <- iot.MbxChargerChargeStatusOn
	}

}
