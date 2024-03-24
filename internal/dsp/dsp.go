package dsp

import (
	"fmt"
	"image/color"
	"log"
	"machine"
	"time"

	"tinygo.org/x/drivers/waveshare-epd/epd4in2"
	"tinygo.org/x/drivers/ws2812"
	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/freemono"
	"tinygo.org/x/tinyfont/gophers"
)

// Content for the Display
type Content struct {
	name      string
	isDirty   bool
	lastClean time.Time
	age       string

	gatewayHeartbeatStatus string
	mbxDoorOpenedStatus    string
	youGotMailIndicator    string
}

// NewContent
func NewContent() *Content {

	content := Content{
		isDirty:                false,
		lastClean:              time.Now(),
		age:                    "0h",
		name:                   "Mailbox IOT",
		gatewayHeartbeatStatus: "initial",
		mbxDoorOpenedStatus:    "initial",
		youGotMailIndicator:    "initial",
	}

	return &content
}

func (content *Content) SetGatewayHeartbeatStatus(status string) {
	if status != content.gatewayHeartbeatStatus {
		// Don't set is dirty just for a heartbeat
		content.gatewayHeartbeatStatus = status
	}
}
func (content *Content) SetMbxDoorOpenedStatus(status string) {

	if status != content.mbxDoorOpenedStatus {
		content.isDirty = true
		content.mbxDoorOpenedStatus = status
		content.youGotMailIndicator = "You got mail!"
	}
}

func (content *Content) SetIsDirty(d bool) {

	log.Printf("internal.dsp.SetIsDirty: %v ", d)
	content.isDirty = d
	content.lastClean = time.Now() // reset
	content.UpdateAge()

}


func (content *Content) UpdateAge() {

	duration := time.Since(content.lastClean)
  age := fmt.Sprintf("%1.1fh", duration.Hours())
	content.age = age

}

func (content *Content) IsDirty() bool {
	return content.isDirty
}

func (content *Content) SetYouGotMailIndicator(i string) {
	content.youGotMailIndicator = i
}

func RunLight(led machine.Pin, count int) {

	// blink run light for a bit seconds so I can tell it is starting
	for i := 0; i < count; i++ {
		led.High()
		time.Sleep(time.Millisecond * 100)
		led.Low()
		time.Sleep(time.Millisecond * 100)
		print("run-")
	}
	print("\n")

}

func ClearDisplay(display *epd4in2.Device) {

	display.ClearBuffer()
	display.ClearDisplay()
	display.WaitUntilIdle()
	log.Println("internal.dsp.ClearDisplay: Waiting for 3 seconds")
	time.Sleep(3 * time.Second)

}

func FontExamples(display *epd4in2.Device) {

	black := color.RGBA{1, 1, 1, 255}
	// white := color.RGBA{0, 0, 0, 255}

	time.Sleep(3 * time.Second)

	// tinyfont.WriteLineRotated(&display, &freemono.Bold9pt7b, 85, 26, "World!", white, tinyfont.ROTATION_180)
	// tinyfont.WriteLineRotated(&display, &freemono.Bold9pt7b, 55, 60, "@tinyGolang", black, tinyfont.ROTATION_90)

	// tinyfont.WriteLineRotated(display, &gophers.Regular58pt, 40, 50, "ABCDEFG\nHIJKLMN\nOPQRSTU", black, tinyfont.NO_ROTATION)
	tinyfont.WriteLineRotated(display, &gophers.Regular58pt, 40, 50, "ABCDEFG\nHIJKLMN\nOPQRSTU\nHH", black, tinyfont.NO_ROTATION)

	// tinyfont.WriteLineColorsRotated(&display, &freemono.Bold9pt7b, 45, 180, "tinyfont", []color.RGBA{white, black}, tinyfont.ROTATION_270)

	log.Println("internal.dsp.FontExamples: Display()")
	display.Display()

	log.Println("internal.dsp.FontExamples: WaitUntilIdle()")
	display.WaitUntilIdle()
	log.Println("internal.dsp.FontExamples: WaitUntilIdle() done.")

}

func (content *Content) DisplayContent(display *epd4in2.Device) {

	log.Println("internal.dsp.DisplayContent: sleep for a bit!")

	black := color.RGBA{1, 1, 1, 255}
	time.Sleep(3 * time.Second)

	stuff := fmt.Sprintf("Gateway HB: %s\n", content.gatewayHeartbeatStatus)
	stuff += fmt.Sprintf("Age: %s\n", content.age)
	stuff += fmt.Sprintf("-------------------------\n\n")
	stuff += fmt.Sprintf("Mbx: %s %s ", content.mbxDoorOpenedStatus, content.youGotMailIndicator)

	// tinyfont.WriteLineRotated(display, &gophers.Regular58pt, 40, 50,  "HH", black, tinyfont.NO_ROTATION)
	tinyfont.WriteLineRotated(display, &freemono.Bold9pt7b, 30, 50, stuff, black, tinyfont.NO_ROTATION)

	log.Println("internal.dsp.DisplayContent: Display()")
	display.Display()

	log.Println("internal.dsp.DisplayContent: WaitUntilIdle()")
	display.WaitUntilIdle()
	log.Println("internal.dsp.DisplayContent: WaitUntilIdle() done.")

}

func NeoNightrider(neo machine.Pin) {

	// Flash a strip of 8
	ws := ws2812.New(neo)
	leds := make([]color.RGBA, 8)
	runStripRGB(leds, neo, &ws)

}

func NeoBlink(neo machine.Pin) {

	// All on then off
	ws := ws2812.New(neo)
	leds := make([]color.RGBA, 8)

	allLedsOn(leds, &ws, 10)
	time.Sleep(100 * time.Millisecond)

	allLedsOff(leds, &ws)
	time.Sleep(10 * time.Millisecond)
}

func runStripRGB(leds []color.RGBA, neo machine.Pin, ws *ws2812.Device) {

	for x := 0; x < 1; x++ {

		// night rider right to left
		for i := 0; i < len(leds); i++ {

			leds[i] = color.RGBA{R: 255, G: 0, B: 0}
			for j := i - 1; j >= 0; j-- {
				leds[j] = color.RGBA{R: 0, G: 0, B: 0}
			}
			ws.WriteColors(leds[:])
			time.Sleep(50 * time.Millisecond)

		}

		allLedsOff(leds, ws)
		time.Sleep(10 * time.Millisecond)

		// night rider left to right
		for i := len(leds) - 1; i >= 0; i-- {

			leds[i] = color.RGBA{R: 255, G: 0, B: 0}
			for j := i + 1; j < len(leds); j++ {
				leds[j] = color.RGBA{R: 0, G: 0, B: 0}
			}
			ws.WriteColors(leds[:])
			time.Sleep(50 * time.Millisecond)

		}

		allLedsOff(leds, ws)
		time.Sleep(10 * time.Millisecond)
	}

	allLedsOff(leds, ws)
}

func allLedsOff(leds []color.RGBA, ws *ws2812.Device) {
	off := color.RGBA{R: 0, G: 0, B: 0}
	setAllLeds(leds, off)
	ws.WriteColors(leds[:])
}

func allLedsOn(leds []color.RGBA, ws *ws2812.Device, intensity uint8) {
	on := color.RGBA{R: intensity, G: intensity, B: intensity}
	setAllLeds(leds, on)
	ws.WriteColors(leds[:])
}

func setAllLeds(leds []color.RGBA, c color.RGBA) {
	for i := 0; i < len(leds); i++ {
		leds[i] = c
	}
}
