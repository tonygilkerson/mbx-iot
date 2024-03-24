//
// This package is used to test the UART Message Bus
///
package main

import (
	"log"
	"machine"
	"os"
	"runtime"
	"time"

	"github.com/tonygilkerson/mbx-iot/internal/umsg"
)

// In this setup UART0 and UART1 are connect together
// so we can perform loop back tests

// Wiring:
// 	GPIO0 -> GPIO5
// 	GPIO1 -> GPIO4

func main() {

	log.Printf("[main] - Startup pause...\n")
	time.Sleep(time.Second * 3)
	log.Printf("[main] - After startup pause\n")

	/////////////////////////////////////////////////////////////////////////////
	// Pins
	/////////////////////////////////////////////////////////////////////////////
	log.Printf("[main] - Setup\n")

	uartIn := machine.UART0
	uartInTx := machine.GPIO0
	uartInRx := machine.GPIO1

	uartOut := machine.UART1
	uartOutTx := machine.GPIO4
	uartOutRx := machine.GPIO5

	/////////////////////////////////////////////////////////////////////////////
	// Broker
	/////////////////////////////////////////////////////////////////////////////

	fooCh := make(chan umsg.FooMsg)
	statusCh := make(chan umsg.StatusMsg)

	mb := umsg.NewBroker(
		"umsg",

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

	/////////////////////////////////////////////////////////////////////////////
	// Tests
	/////////////////////////////////////////////////////////////////////////////

	fooTest(&mb, fooCh)
	iotStatusTest(&mb, statusCh)

	// Done
	log.Printf("[main] - **** DONE ****")
	os.Exit(0)
}


func fooTest(mb *umsg.MsgBroker, fooCh chan umsg.FooMsg){

	var fm umsg.FooMsg
	fm.Kind = umsg.MSG_FOO
	fm.SenderID = umsg.LOOKBACK_SENDERID
	fm.Name = "This is a foo message from loopback"

	log.Printf("[fooTest] - PublishFoo(fm)\n")
	mb.PublishFooToUart(fm)

	var found bool = false
	var msg umsg.FooMsg

	// Non-blocking ch read that will timeout... boom!
	boom := time.After(3000 * time.Millisecond)
	for {
		select {
		case msg = <-fooCh:
			found = true
		case <-boom:
			log.Printf("[fooTest] - Boom! timeout waiting for message\n")
			break
		default:
			runtime.Gosched()
			time.Sleep(50 * time.Millisecond)
		}

		if found {
			break
		}
	}

	log.Printf("[fooTest] - ******************************************************************\n")
	if found {
		if msg.Name == fm.Name {
			log.Printf("[fooTest] - SUCCESS, msg: [%v]\n", msg)
		} else {
			log.Printf("[fooTest] - FAIL, wrong msg: [%v]\n", msg)
		}
	} else {
		log.Printf("[fooTest] - FAIL, did not receive message.")
	}
	log.Printf("[fooTest] - ******************************************************************\n")

}

func iotStatusTest(mb *umsg.MsgBroker, statusCh chan umsg.StatusMsg){

	var statusMsg umsg.StatusMsg
	statusMsg.Kind = umsg.MSG_STATUS
	statusMsg.SenderID = umsg.LOOKBACK_SENDERID
	statusMsg.Key = "This is a status key"
	statusMsg.Value = "This is status value"

	log.Printf("iotStatusTest: PublishIosStatus(statusMsg)\n")
	mb.PublishStatusToUart(statusMsg)

	var found bool = false
	var msg umsg.StatusMsg

	// Non-blocking ch read that will timeout... boom!
	boom := time.After(3000 * time.Millisecond)
	for {
		select {
		case msg = <-statusCh:
			found = true
		case <-boom:
			log.Printf("iotStatusTest: Boom! timeout waiting for message\n")
			break
		default:
			runtime.Gosched()
			time.Sleep(50 * time.Millisecond)
		}

		if found {
			break
		}
	}

	log.Printf("iotStatusTest: ******************************************************************\n")
	if found {
		if msg.Key == statusMsg.Key {
			log.Printf("iotStatusTest: SUCCESS, msg: [%v]\n", msg)
		} else {
			log.Printf("iotStatusTest: FAIL, wrong msg: [%v]\n", msg)
		}
	} else {
		log.Printf("iotStatusTest: FAIL, did not receive message.")
	}
	log.Printf("iotStatusTest: ******************************************************************\n")

}
