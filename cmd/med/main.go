package main

import (
	"fmt"
	"log"
	"machine"
	"runtime"
	"time"

	"image/color"

	"github.com/tonygilkerson/mbx-iot/internal/dsp"
	"github.com/tonygilkerson/mbx-iot/internal/med"
	"tinygo.org/x/drivers/st7789"
	"tinygo.org/x/drivers/tone"
	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/freemono"
)

func main() {
	//
	// Named PINs
	//
	var tookMedsButton machine.Pin = machine.GP2
	var sub1HrButton machine.Pin = machine.GP3
	var buzzerPin machine.Pin = machine.GP7
	var dspDC machine.Pin = machine.GP8
	var dspCS machine.Pin = machine.GP9
	var dspSCK machine.Pin = machine.GP10
	var dspSDO machine.Pin = machine.GP11
	var dspReset machine.Pin = machine.GP12
	var dspBackLight machine.Pin = machine.GP13
	var add1HrButton machine.Pin = machine.GP15
	var add30MButton machine.Pin = machine.GP17
	var dspSDI machine.Pin = machine.GP28

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
		log.Panicln("failed to configure buzzer")
	}

	//
	// Med Device
	//
	medTracker := med.New(add1HrButton, sub1HrButton, add30MButton, tookMedsButton, buzzer)
	go medTracker.KeyPressChannelConsumer()

	//
	// Display
	//
	machine.SPI1.Configure(machine.SPIConfig{
		Frequency: 8000000,
		LSBFirst:  false,
		Mode:      0,
		SCK:       dspSCK,
		SDO:       dspSDO,
		SDI:       dspSDI, // I don't think this is actually used for LCD, just assign to any open pin
	})

	display := st7789.New(machine.SPI1,
		dspReset,     // TFT_RESET
		dspDC,        // TFT_DC
		dspCS,        // TFT_CS
		dspBackLight) // TFT_LITE

	display.Configure(st7789.Config{
		// With the display in portrait and the usb socket on the left and in the back
		// the actual width and height are switched width=320 and height=240
		Width:        240,
		Height:       320,
		Rotation:     st7789.ROTATION_90,
		RowOffset:    0,
		ColumnOffset: 0,
		FrameRate:    st7789.FRAMERATE_111,
		VSyncLines:   st7789.MAX_VSYNC_SCANLINES,
	})

	//
	// Start
	//
	log.Printf("start")

	width, height := display.Size()
	log.Printf("width: %v, height: %v\n", width, height)

	// red := color.RGBA{126, 0, 0, 255} // dim
	red := color.RGBA{255, 0, 0, 255}
	// black := color.RGBA{0, 0, 0, 255}
	// white := color.RGBA{255, 255, 255, 255}
	// blue := color.RGBA{0, 0, 255, 255}
	// green := color.RGBA{0, 255, 0, 255}

	// screenOnAt := time.Now()
	// screenOn := true

	/////////////////////////////////////////////////////////////////////////////
	// The main loop
	/////////////////////////////////////////////////////////////////////////////
	for {

		if medTracker.CheckIfKeyPressed() {
			// Screen on
			display.Sleep(false)
			dspBackLight.High()
		} else {
			// Screen off
			display.Sleep(true)
			dspBackLight.Low()
		}

		// if !screenOn {
		// 	display.Sleep(false)
		// 	dspBackLight.High()
		// 	screenOn = false
		// 	screenOnAt = time.Now()
		// 	break
		// }

		lastTakenMedsDuration := time.Since(medTracker.GetLastTakenMedsAt())
		age := fmt.Sprintf("%1.2fh", lastTakenMedsDuration.Hours())
		ageString := fmt.Sprintf("Last taken:\n%s hours ago", age)

		cls(&display)
		// tinyfont.WriteLine(&display,&freemono.Regular12pt7b,10,20,"123456789-123456789-x",red)
		tinyfont.WriteLine(&display, &freemono.Regular12pt7b, 10, 20, ageString, red)
		time.Sleep(time.Second * 3)

		//test
		// soundSiren(buzzer)

		// screenOnDuration := time.Since(screenOnAt)
		// if screenOn && screenOnDuration.Minutes() > 1 {
		// 	//turn off the screen
		// 	// GP12 - LCD_RST (low active)
		// 	// GP13 - LCD_BL
		// 	log.Println("Turn off screen")
		// 	display.Sleep(true)
		// 	dspBackLight.Low()
		// 	screenOn = false
		// }

		runtime.Gosched()
		time.Sleep(time.Millisecond * 1000)
		log.Println(".")
	}

}

/////////////////////////////////////////////////////////////////////////////
// fn
/////////////////////////////////////////////////////////////////////////////

func paintScreen(c color.RGBA, d *st7789.Device, s int16) {
	var x, y int16
	for y = 0; y < 240; y = y + s {
		for x = 0; x < 320; x = x + s {
			d.FillRectangle(x, y, s, s, c)
		}
	}
}

func cls(d *st7789.Device) {
	black := color.RGBA{0, 0, 0, 255}
	d.FillScreen(black)
	fmt.Printf("FillScreen(black)\n")
}

func soundSiren(buzzer tone.Speaker) {
	for i := 0; i < 1; i++ {
		log.Println("nee")
		buzzer.SetNote(tone.B5)
		time.Sleep(time.Second / 2)

		log.Println("naw")
		buzzer.SetNote(tone.A5)
		time.Sleep(time.Second / 2)

	}
	buzzer.Stop()
}
