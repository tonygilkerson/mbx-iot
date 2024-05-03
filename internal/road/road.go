package road

import (
	"log"
	"machine"
	"runtime"
	"strings"
	"time"

	"tinygo.org/x/drivers/lora"
	"tinygo.org/x/drivers/sx127x"
)

type CommunicationMode int

// TxOnly
// The Lora protocol wants you to do both Rx and Tx but if you are a device who's main purpose is to
// collect and send data only, then the process can be optimized if you set its mode to TxOnly
// For TxOnly if there is no data to send then the Lora RxTx cycle can be skipped.
//
// TxRx
// If you are a device that need to only receive data, then you have no choice but to do a TxRx every cycle
// Do a Tx and Rx each cycle
const (
	TxRx CommunicationMode = iota
	TxOnly
)

type Radio struct {
	SPI               machine.SPI
	EN                machine.Pin
	RST               machine.Pin
	CS                machine.Pin
	DIO0              machine.Pin
	DIO1              machine.Pin
	SCK               machine.Pin
	SDO               machine.Pin
	SDI               machine.Pin
	SxDevice          *sx127x.Device
	TxQ               *chan string
	RxQ               *chan string
	RxTimeoutMs       uint32
	TxTimeoutMs       uint32
	TxRxLoopTickerSec uint32
	CommunicationMode CommunicationMode
}

//
// DEVTODO - Not sure if/how this is used. I am going to comment out and see what happens
//           If it is needed then I will need to move it to main
//
// func dioIrqHandler(machine.Pin) {
// 	loraRadio.HandleInterrupt()
// }

// setupLora will setup the lora radio device
func SetupLora(
	spi machine.SPI,
	en machine.Pin,
	rst machine.Pin,
	cs machine.Pin,
	dio0 machine.Pin,
	dio1 machine.Pin,
	sck machine.Pin,
	sdo machine.Pin,
	sdi machine.Pin,
	sxDevice *sx127x.Device,
	txQ *chan string,
	rxQ *chan string,
	txTimeoutMs uint32,
	rxTimeoutMs uint32,
	txRxLoopTickerSec uint32,
	CommunicationMode CommunicationMode,
) Radio {

	//
	// Populate Radio props
	//

	var radio Radio
	radio.SPI = spi
	radio.EN = en
	radio.RST = rst
	radio.CS = cs
	radio.DIO0 = dio0
	radio.DIO1 = dio1
	radio.SCK = sck
	radio.SDO = sdo
	radio.SDI = sdi

	if rxTimeoutMs == 0 {
		radio.RxTimeoutMs = 5_000
	} else {
		radio.RxTimeoutMs = rxTimeoutMs
	}

	if txTimeoutMs == 0 {
		radio.TxTimeoutMs = 10_000
	} else {
		radio.TxTimeoutMs = txTimeoutMs
	}

	if txRxLoopTickerSec == 0 {
		radio.TxRxLoopTickerSec = 1
	} else {
		radio.TxRxLoopTickerSec = txRxLoopTickerSec
	}

	radio.CommunicationMode = CommunicationMode

	spi.Configure(machine.SPIConfig{
		SCK: sck,
		SDO: sdo,
		SDI: sdi,
	})

	spi.Configure(machine.SPIConfig{Frequency: 500000, Mode: 0})
	rst.Configure(machine.PinConfig{Mode: machine.PinOutput})
	en.Configure(machine.PinConfig{Mode: machine.PinOutput})
	en.High() // enable the radio by default

	radio.SxDevice = sx127x.New(spi, rst)
	radio.SxDevice.SetRadioController(sx127x.NewRadioControl(cs, dio0, dio1))
	radio.SxDevice.Reset()
	state := radio.SxDevice.DetectDevice()
	if !state {
		panic("main: sx127x NOT FOUND !!!")
	} else {
		log.Println("road: sx127x found")
	}

	// Prepare for Lora Operation
	loraConf := lora.Config{
		Freq:           lora.MHz_916_8,
		Bw:             lora.Bandwidth_125_0,
		Sf:             lora.SpreadingFactor9,
		Cr:             lora.CodingRate4_7,
		HeaderType:     lora.HeaderExplicit,
		Preamble:       12,
		Iq:             lora.IQStandard,
		Crc:            lora.CRCOn,
		SyncWord:       lora.SyncPrivate,
		LoraTxPowerDBm: 20,
	}

	radio.SxDevice.LoraConfig(loraConf)

	radio.TxQ = txQ
	radio.RxQ = rxQ
	return radio
}

// SplitMessageBatch will split a batch of messages and return a slice of messages
// All messages sent over the radio are batched for efficiency
// Messages on the queue are stored as strings separated by a pipe character "|"
// For example: message batch -> "msg1|msg2|msg3|..."
func SplitMessageBatch(msgBatch string) []string {
	return strings.Split(string(msgBatch), "|")
}

func (radio *Radio) LoraRxTxRunner() {
	log.Printf("road.LoraRxTxRunner: with TxRxLoopTickerSec: %v",radio.TxRxLoopTickerSec)

	ticker := time.NewTicker(time.Second * time.Duration(radio.TxRxLoopTickerSec))
	for range ticker.C {
		log.Println("road.LoraRxTxRunner: call LoraRxTx()...")
		radio.LoraRxTx()
		runtime.Gosched()
	}

}



func (radio *Radio) LoraRxTx() (rxData bool) {
	txQ := radio.TxQ
	rxQ := radio.RxQ

	// Did we get any data from the Rx?
	rxData = false

	//
	// If there are no messages in the channel then get out quick
	//
	if radio.CommunicationMode == TxOnly && len(*txQ) == 0 {
		log.Println("road.LoraRxTx: txQ is empty, mode=TxOnly so getting out early...")
		return rxData
	}

	// Enable the radio
	radio.EN.High()

	//
	// RX - Receive
	//
	log.Println("road.LoraRxTx: RX Start - Receiving")
	buf, err := radio.SxDevice.Rx(radio.RxTimeoutMs)

	if err != nil {
		log.Println("road.LoraRxTx: RX Error: ", err)

	} else if buf != nil {

		log.Printf("road.LoraRxTx: RX Packet Received: [%v]", string(buf))
		rxData = true

		// Use non-blocking send so if the channel buffer is full,
		// the value will get dropped instead of crashing the system
		select {
		case *rxQ <- string(buf):
		default:
		}

	} else {
		log.Println("road.LoraRxTx: RX nothing to receive")
	}

	//
	// Batch - batch all message in txQ
	//
	var batchMsg string

	// Concatenate all messages separated by \n
	eom := false //end of messages
	for {
		select {
		case msg := <-*txQ:
			if len(batchMsg) > 0 {
				batchMsg = batchMsg + "|" + msg
			} else {
				batchMsg = msg
			}
		default:
			eom = true
		}

		// break out if end of messages
		if eom {
			break
		}
	}

	//
	// TX - Send batch
	//
	if len(batchMsg) > 0 {
		log.Printf("road.LoraRxTx: TX [%v]", batchMsg)
		err := radio.SxDevice.Tx([]byte(batchMsg), radio.TxTimeoutMs)
		if err != nil {
			log.Printf("road.LoraRxTx: TX Error [%v]", err)
		}
	} else {
		log.Println("road.LoraRxTx: TX nothing to send, skipping TX")
	}

	// Disable the radio to save power...
	radio.EN.Low()

	return rxData
}
