/*
umsg - UART messaging

# Once configured the user of this package can publish message message to other devices via UART

DEVTODO - describe how devices are connected with one uart use for input and the other uart used for output
  - describe how the devices are expected to make a loop so that a message sent is forwarded around the loop until returns to the sender
  - describe how users can subscribe to messages by listening to specific message queues (aka channels)
*/
package umsg

import (
	"log"
	"machine"
	"runtime"
	"strings"
	"time"
)

// Define message types
type MsgType string

const (
	TOKEN_HAT         byte   = 94  // ^
	TOKEN_ABOUT       byte   = 126 // ~
	TOKEN_PIPE        byte   = 124 // |
	LOOKBACK_SENDERID string = "Loopback"
)

const (
	MSG_FOO    MsgType = "Foo"
	MSG_STATUS MsgType = "Status"
)

// ^Foo|some-sender|This is a foo message~
type FooMsg struct {
	Kind     MsgType
	SenderID string
	Name     string
}

// ^Status|some-sender|somekey|somevalue~
// ^Status|test|GatewayHeartbeat|1234~
type StatusMsg struct {
	Kind     MsgType
	SenderID string
	Key      string
	Value    string
}

type MsgInterface interface {
	FooMsg | StatusMsg
}

type UART interface {
	Configure(config machine.UARTConfig) error
	Buffered() int
	ReadByte() (byte, error)
	Write(data []byte) (n int, err error)
}

// Message Broker
type MsgBroker struct {
	// Sender ID set on each message
	// If a sender receives its own message it will not be forwarded on
	senderID string

	uartIn      UART
	uartInTxPin machine.Pin
	uartInRxPin machine.Pin

	uartOut      UART
	uartOutTxPin machine.Pin
	uartOutRxPin machine.Pin

	fooCh    chan FooMsg
	statusCh chan StatusMsg
}

func NewBroker(
	senderID string,

	uartIn UART,
	uartInTxPin machine.Pin,
	uartInRxPin machine.Pin,

	uartOut UART,
	uartOutTxPin machine.Pin,
	uartOutRxPin machine.Pin,

	fooCh chan FooMsg,
	statusCh chan StatusMsg,

) MsgBroker {

	var mb MsgBroker

	mb.senderID = senderID

	if uartIn != nil {
		mb.uartIn = uartIn
		mb.uartInTxPin = uartInTxPin
		mb.uartInRxPin = uartInRxPin
	}

	if uartOut != nil {
		mb.uartOut = uartOut
		mb.uartOutTxPin = uartOutTxPin
		mb.uartOutRxPin = uartOutRxPin
	}

	if fooCh != nil {
		mb.fooCh = fooCh
	}

	if statusCh != nil {
		mb.statusCh = statusCh
	}

	return mb

}

func (mb *MsgBroker) Configure() {

	// Output UART
	if mb.uartOut != nil {
		mb.uartOut.Configure(machine.UARTConfig{TX: mb.uartOutTxPin, RX: mb.uartOutRxPin})
	}

	// Input UART
	if mb.uartIn != nil {
		mb.uartIn.Configure(machine.UARTConfig{TX: mb.uartInTxPin, RX: mb.uartInRxPin})
	}

}

// DEVTODO - make this generic
func (mb *MsgBroker) PublishFooToUart(foo FooMsg) {

	if foo.SenderID == "" {
		foo.SenderID = mb.senderID
	}

	msgStr := "^" + string(foo.Kind)
	msgStr = msgStr + "|" + foo.SenderID
	msgStr = msgStr + "|" + foo.Name + "~"

	mb.writeMsgToUart(msgStr)

}

func (mb *MsgBroker) PublishStatusToUart(sm StatusMsg) {

	if sm.SenderID == "" {
		sm.SenderID = mb.senderID
	}

	msgStr := "^" + string(sm.Kind)
	msgStr = msgStr + "|" + sm.SenderID
	msgStr = msgStr + "|" + sm.Key
	msgStr = msgStr + "|" + sm.Value + "~"

	mb.writeMsgToUart(msgStr)

}

func (mb *MsgBroker) writeMsgToUart(msg string) {

	if mb.uartOut != nil {
		mb.uartOut.Write([]byte(msg))
		// Print a new line between messages for readability in the serial monitor
		mb.uartOut.Write([]byte("\n"))
	}

}

func (mb *MsgBroker) dispatchMsgToChannel(msgParts []string) {

	switch msgParts[0] {

	case string(MSG_FOO):
		log.Printf("umsg.dispatchMsgToChannel: %v\n", MSG_FOO)
		msg := makeFoo(msgParts)

		if mb.fooCh != nil {
			mb.fooCh <- *msg
		}

	case string(MSG_STATUS):
		log.Printf("umsg.dispatchMsgToChannel: %v\n", MSG_STATUS)
		msg := makeStatus(msgParts)
		if mb.statusCh != nil {
			mb.statusCh <- *msg
		}

	default:
	}

}

//
// UartReader will read all the messages in the buffer. It will keep reading as long as there is data in the buffer
//
//	the function will return when there is no more data in the buffer
func (mb *MsgBroker) UartReader() {

	for {
		//
		// Loop delay to ensure we don't hog the cpu
		//
		log.Println("umsg.UartReader: quick pause")
		runtime.Gosched()
		time.Sleep(time.Millisecond * 10)

		//
		// Peak to see if we have data in the buffer
		//
		if mb.uartIn.Buffered() == 0 {
			log.Println("umsg.UartReader: no data in uart buffer, get out!")
			return
		}

		//
		// Read a message from the buffer
		//
		msg := mb.readMsg()
		if len(msg) == 0 {
			log.Println("umsg.UartReader: empty message, get out!")
			continue
		}

		msgParts := strings.Split(string(msg), "|")

		//
		// Make sure the message has values, message look like:
		//      kind|senderId|field1|field2...
		//
		if len(msgParts) < 2 {
			log.Println("umsg.UartReader: message had no values, get out! msg: %s", msg)
			continue
		}

		// Get the message senderID, it is assumed that index 1 is sender id
		msgSenderID := msgParts[1]

		// Only dispatch messages from other senders
		// This can happen when a message makes it way around the loop and arrives back at the original sender
		if msgSenderID != mb.senderID {

			mb.dispatchMsgToChannel(msgParts)

			// Forward all messages with the exception of the loopback sender to prevent endless loop
			// The loopback is mainly used for testing. It allows you point uartOut->UartIn on the same pico
			if mb.uartOut != nil && msgSenderID != LOOKBACK_SENDERID {
				// rewrap the message to start with ^ and end with ~
				msg = string(TOKEN_HAT) + msg + string(TOKEN_ABOUT)
				log.Printf("umsg.UartReader: send message to output uart: %v\n", msg)
				mb.uartOut.Write([]byte(msg))
			}

		}

	}

}

/*
readMsg will read the input buffer looking for a message

Given:

	this-is-junk^Foo|some-sender|This is a foo message~^Bar|some-sender|This is a bar message~more-junk

The following string is returned:

	Foo|some-sender|This is a foo message

The next time readMsg() is called this is returned:

	Bar|some-sender|This is a bar message
*/
func (mb *MsgBroker) readMsg() (msg string) {

	// used to hold message read from input UART
	message := make([]byte, 0)

	// Seek receive buffer to start of next message
	// if no message is found then get out
	if !mb.seekStartOfMessage() {
		log.Println("umsg.readMsg: did not find start of message")
		return ""
	}

	//
	// Start read message loop
	//
	log.Printf("umsg.readMsg: Start message read loop")
	for {

		// No data buffered so get out
		if mb.uartIn.Buffered() == 0 {
			log.Printf("umsg.readMsg: No data in buffer, get out!")
			return ""
		}

		// Read from buffer
		data, err := mb.uartIn.ReadByte()

		if err != nil {
			log.Printf("umsg.readMsg: error reading uart buffer, get out!")
			return ""
		}

		// Unexpected start of next message
		if data == TOKEN_HAT {
			log.Printf("umsg.readMsg: Unexpected start of message, discarding partial message and continuing to read next message. Partial message %s", string(message))
			message = nil
			continue
		}

		// End of message
		if data == TOKEN_ABOUT {
			log.Printf("umsg.readMsg: EOM break!")
			break
		}

		// Build a message up byte by byte
		message = append(message, data)
	}

	// Set return values
	if len(message) > 0 {
		log.Printf("umsg.readMsg: return this message:  %v\n", string(message))
		return string(message)
	} else {
		log.Printf("umsg.readMsg: return empty message\n")
		return ""
	}

}

// seekStartOfMessage will read data from the buffer one byte at a time until it find the start of a message
//
//	all data found before the start of message character `^` goes to the bit bucket
func (mb *MsgBroker) seekStartOfMessage() (isFound bool) {

	for {
		// No data buffered so get out
		if mb.uartIn.Buffered() < 1 {
			return false
		}

		data, err := mb.uartIn.ReadByte()

		// if we hit end of buffer before we find message return not found
		if err != nil {
			log.Printf("umsg.seekStartOfMessage: return because we hit an err: %v", err)
			return false
		}

		// the '^' character is the start of a message
		if data == TOKEN_HAT {
			return true
		}

	}

}

// DEVTODO - make this generic
func makeFoo(msgParts []string) *FooMsg {

	fooMsg := new(FooMsg)

	if len(msgParts) > 0 {
		fooMsg.Kind = MSG_FOO
	}
	if len(msgParts) > 1 {
		fooMsg.SenderID = msgParts[1]
	}
	if len(msgParts) > 2 {
		fooMsg.Name = msgParts[2]
	}

	return fooMsg
}

func makeStatus(msgParts []string) *StatusMsg {

	statusMsg := new(StatusMsg)

	if len(msgParts) > 0 {
		statusMsg.Kind = MSG_STATUS
	}
	if len(msgParts) > 1 {
		statusMsg.SenderID = msgParts[1]
	}
	if len(msgParts) > 2 {
		statusMsg.Key = msgParts[2]
	}
	if len(msgParts) > 3 {
		statusMsg.Value = msgParts[3]
	}

	return statusMsg
}
