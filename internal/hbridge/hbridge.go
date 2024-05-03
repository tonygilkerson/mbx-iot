package hbridge

import (
	"machine"
	"time"
	"fmt"
)

//
// Device is a L293D IC
//
type Device struct {
	enable machine.Pin
	in1 machine.Pin
	in2 machine.Pin
	lastTurnOnTime time.Time
	// Currently only using the left channel thus I will not define in3 & in4 until I need them
}

// New creates an instance of a L293D device
func New(enable machine.Pin, in1 machine.Pin, in2 machine.Pin) *Device {

	enable.Configure(machine.PinConfig{Mode: machine.PinOutput})
	in1.Configure(machine.PinConfig{Mode: machine.PinOutput})
	in2.Configure(machine.PinConfig{Mode: machine.PinOutput})

	return &Device{
		enable: enable,
		in1: in1,
		in2: in2,
		lastTurnOnTime: time.Now(),
	}
}

// Off turns off power, does not push or pull and the motor does not rotate
func (d *Device) Off(){
	d.enable.Low()
	d.in1.Low()
	d.in2.Low()
}

// TurnOn means push solenoid rod or rotate motor CW
func (d *Device) TurnOn(){
	d.cw()
	time.Sleep(time.Duration(time.Second * 1))
	d.Off()
}

// TurnOn means pull solenoid rod or rotate motor CCW
func (d *Device) TurnOff(){
	d.ccw()
	time.Sleep(time.Duration(time.Second * 1))
	d.Off()
}

// cw means push solenoid rod or rotate motor CW
func (d *Device) cw(){
	d.enable.High()
	d.in1.High()
	d.in2.Low()
}

// ccw means pull solenoid rod or rotate motor CCW
func (d *Device) ccw(){
	d.enable.High()
	d.in1.Low()
	d.in2.High()
}

// GetTurnOnAge returns the age since last TurnOn as a HH.hh string where hh is a fraction of an hour
// for example, 3.5 is 3 hours and 30 minutes
func (d *Device) GetTurnOnAge() string {

	duration := time.Since(d.lastTurnOnTime)
  age := fmt.Sprintf("%0.2f", duration.Hours())
	return age

}
