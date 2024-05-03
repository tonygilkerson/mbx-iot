package main

import (
	"fmt"
	"log"
	"machine"
	"runtime"
	"strconv"
	"strings"
	"time"

	tm1637mod "github.com/tonygilkerson/mbx-iot/hack/driver/tm1637"
	"github.com/tonygilkerson/mbx-iot/internal/dsp"
	"github.com/tonygilkerson/mbx-iot/internal/hbridge"
	"github.com/tonygilkerson/mbx-iot/internal/road"
	"github.com/tonygilkerson/mbx-iot/internal/soil"
	"github.com/tonygilkerson/mbx-iot/internal/util"
	"github.com/tonygilkerson/mbx-iot/pkg/iot"

	"tinygo.org/x/drivers/sx127x"
)

func main() {

	//
	// Named PINs
	//
	var vibrationPin machine.Pin = machine.GP5
	var hBridgeEnable machine.Pin = machine.GP6
	var hBridgeIn1 machine.Pin = machine.GP7
	var hBridgeIn2 machine.Pin = machine.GP8
	var tm1637CLK machine.Pin = machine.GP10
	var tm1637DIO machine.Pin = machine.GP11
	var soilSDA machine.Pin = machine.GP12
	var soilSCL machine.Pin = machine.GP13
	var loraEn machine.Pin = machine.GP15
	var loraSdi machine.Pin = machine.GP16 // machine.SPI0_SDI_PIN
	var loraCs machine.Pin = machine.GP17
	var loraSck machine.Pin = machine.GP18 // machine.SPI0_SCK_PIN
	var loraSdo machine.Pin = machine.GP19 // machine.SPI0_SDO_PIN
	var loraRst machine.Pin = machine.GP20
	var loraDio0 machine.Pin = machine.GP21 // (GP21--G0) Must be connected from pico to breakout for radio events IRQ to work
	var loraDio1 machine.Pin = machine.GP22 // (GP22--G1) I don't now what this does but it seems to need to be connected

	var led machine.Pin = machine.GPIO25 // GP25 machine.LED

	const (
		HEARTBEAT_DURATION_SECONDS = 600
	)

	//
	// run light
	//
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})
	dsp.RunLight(led, 10)
	// log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetFlags(log.LstdFlags | log.Llongfile)

	//
	// Configure the button
	//
	chVibration := make(chan string, 1)
	// vibrationPin.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	vibrationPin.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	vibrationPin.SetInterrupt(machine.PinRising, func(p machine.Pin) {
		// Use non-blocking send so if the channel buffer is full,
		// the value will get dropped instead of crashing the system
		select {
		case chVibration <- "rise":
		default:
		}

	})

	//
	// Configure L293D
	//
	log.Println("Configure L293D Pins")
	hbridge := hbridge.New(hBridgeEnable, hBridgeIn1, hBridgeIn2)

	//
	// Configure 4 digit 7-segment display
	//
	tm := tm1637mod.New(tm1637CLK, tm1637DIO, 5)

	//
	// Configure I2C
	//
	i2c := machine.I2C0
	err := i2c.Configure(machine.I2CConfig{SDA: soilSDA, SCL: soilSCL})
	util.DoOrDie(err)

	soil := soil.New(i2c)

	//
	// 	Setup Lora
	//
	var loraRadio *sx127x.Device
	txQ := make(chan string, 250) // I would hope the channel size would never be larger than ~4 so 250 is large
	rxQ := make(chan string, 250)

	log.Println("Setup LORA")
	radio := road.SetupLora(
		*machine.SPI0,
		loraEn,
		loraRst,
		loraCs,
		loraDio0,
		loraDio1,
		loraSck,
		loraSdo,
		loraSdi,
		loraRadio,
		&txQ,
		&rxQ,
		10_000,
		10_000,
		HEARTBEAT_DURATION_SECONDS+1, // rule of thumb HEARTBEAT_DURATION_SECONDS + 1
		road.TxOnly)

	// Routine to send and receive
	go radio.LoraRxTxRunner()

	//
	// Main loop
	//
	lastSoilReading := time.Now()
	var showMarquee bool = true
	var moistureReading uint16
	var temperatureReading float64

	var hbridgeToggel bool

	for {

		//
		// Time for a Soil Reading?
		//
		if time.Since(lastSoilReading) > (time.Second * HEARTBEAT_DURATION_SECONDS) {

			lastSoilReading = time.Now()
			log.Printf("------------------SoilMainLoopHeartbeat--------------------")

			// Send Heartbeat to Tx queue
			txQ <- iot.SoilMainLoopHeartbeat
			dsp.RunLight(led, 2)

			// Send Moisture reading  to Tx queue
			moistureReading, err = soil.ReadMoisture()
			util.DoOrDie(err)
			txQ <- fmt.Sprintf("%v:%v", iot.SoilMoisture, moistureReading)
			time.Sleep(time.Second)

			// Send Temperature(F) to Tx queue
			temperatureReading, err = soil.ReadTemperature()
			util.DoOrDie(err)
			txQ <- fmt.Sprintf("%v:%v", iot.SoilTemperature, temperatureReading)
			time.Sleep(time.Second)

			// hbridge not used yet
			hbridge.Off()

		}

		// hbridge test, toggle it on and off each time
		hbridgeToggel = !hbridgeToggel
		if hbridgeToggel {
			log.Println("HBRIDGE test [ ON ]")
			hbridge.TurnOn()
		} else {
			log.Println("HBRIDGE test [ OFF ]")
			hbridge.TurnOff()
		}

		//
		// Vibration sensor tripped
		//
		select {
		case <-chVibration:
			showMarquee = true
		default:
		}

		//
		// Marquee Display
		//
		if showMarquee {
			// moisture
			log.Printf("Moisture: %v\n", moistureReading)
			tm.DisplayNumber(int16(moistureReading))
			time.Sleep(time.Second * 2)
			// temperature
			log.Printf("Temperature (F): %v\n", temperatureReading)
			tm.DisplayNumber(int16(temperatureReading))
			time.Sleep(time.Second * 2)
			// age
			age := hbridge.GetTurnOnAge()
			ageParts := strings.Split(age, ".")
			h, _ := strconv.Atoi(ageParts[0])
			m, _ := strconv.Atoi(ageParts[1])
			log.Printf("Age: %v h: %v m: %v\n", age, h, m)
			tm.DisplayClock(uint8(h), uint8(m), true)
			time.Sleep(time.Second * 2)

			tm.ClearDisplay()
			showMarquee = false
		}

		//
		// Let someone else have a turn
		//
		runtime.Gosched()
		time.Sleep(time.Second * 3)

	}

}
