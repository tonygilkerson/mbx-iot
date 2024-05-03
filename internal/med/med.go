package med

import (
	"log"
	"machine"
	"runtime"
	"time"

	"tinygo.org/x/drivers/tone"
)

type MedTracker struct {
	lastTakenMedsAt time.Time
	add1HrButton    machine.Pin
	sub1HrButton    machine.Pin
	add30MButton    machine.Pin
	tookMedsButton  machine.Pin
	buzzerPin       machine.Pin
	buzzer          tone.Speaker
	chKeyPress      chan string
	keyPressed      bool
}

func New(
	add1HrButton machine.Pin,
	sub1HrButton machine.Pin,
	add30MButton machine.Pin,
	tookMedsButton machine.Pin,
	buzzer tone.Speaker,

) *MedTracker {

	var mt MedTracker

	mt.lastTakenMedsAt = time.Now()
	mt.keyPressed = true

	// assign channel
	mt.chKeyPress = make(chan string, 1)

	// assign buttons
	mt.add1HrButton = add1HrButton
	mt.sub1HrButton = sub1HrButton
	mt.add30MButton = add30MButton
	mt.tookMedsButton = tookMedsButton
	mt.buzzer = buzzer

	// configure buttons
	mt.add1HrButton.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	mt.add30MButton.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	mt.tookMedsButton.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	mt.sub1HrButton.Configure(machine.PinConfig{Mode: machine.PinInputPullup})

	mt.add1HrButton.SetInterrupt(machine.PinFalling, func(p machine.Pin) {
		// Use non-blocking send so if the channel buffer is full,
		// the value will get dropped instead of crashing the system
		select {
		case mt.chKeyPress <- "add1HrButton":
		default:
		}
	})

	mt.add30MButton.SetInterrupt(machine.PinFalling, func(p machine.Pin) {
		select {
		case mt.chKeyPress <- "add30MButton":
		default:
		}
	})

	mt.tookMedsButton.SetInterrupt(machine.PinFalling, func(p machine.Pin) {
		select {
		case mt.chKeyPress <- "tookMedsButton":
		default:
		}
	})

	mt.sub1HrButton.SetInterrupt(machine.PinFalling, func(p machine.Pin) {
		select {
		case mt.chKeyPress <- "sub1HrButton":
		default:
		}
	})

	// return it
	return &mt
}

func (mt *MedTracker) CheckIfKeyPressed() bool {
	keyPressed := mt.keyPressed
	mt.keyPressed = false
	return keyPressed
}

func (mt *MedTracker) GetLastTakenMedsAt() time.Time {
	return mt.lastTakenMedsAt
}

func (mt *MedTracker) KeyPressChannelConsumer() {

	for {
		select {
		case key := <-mt.chKeyPress:
			// The first key press just wakes the screen
			if !mt.keyPressed {
				mt.keyPressed = true
				break
			}

			switch key {
			case "add1HrButton":
				log.Println("med.chKeyPressConsumer: add1HrButton - Add 1hr")
				mt.lastTakenMedsAt = mt.lastTakenMedsAt.Add(time.Hour)
			case "add30MButton":
				log.Println("med.chKeyPressConsumer: add30MButton - Add 30min")
				mt.lastTakenMedsAt = mt.lastTakenMedsAt.Add(time.Minute * 30)
			case "tookMedsButton":
				log.Println("med.chKeyPressConsumer: tookMedsButton - took meds")
				mt.lastTakenMedsAt = time.Now()
			case "sub1HrButton":
				log.Println("med.chKeyPressConsumer: sub1HrButton pressed - Subtract 1hr")
				mt.lastTakenMedsAt = mt.lastTakenMedsAt.Add(time.Hour * -1)
			}

		default:
			runtime.Gosched()
			time.Sleep(time.Millisecond * 250)
			log.Println(".")
		}
	}
}
