/*
	Package via6522 emulates MOS Technology 6522, or the modern WDC 65C22.
	This is a Versatile Interface Adapter (VIA) I/O controller
	designed for use with the 6502 microprocessor.

	The 4-bit RS (register select) is exposed as 16 bytes of address-space.  The
	processor chooses the register using four bits of the 16-bit address bus and
	reads/writes using the 8-bit data bus.

	Peripheral ports

	The W65C22 includes functions for programmed control of two peripheral ports
	(Ports A and B). Two program controlled 8-bit bidirectional peripheral I/O
	ports allow direct interfacing between the microprocessor and selected
	peripheral units. Each port has input data latching capability. Two
	programmable Data Direction Registers (A and B) allow selection of data
	direction (input or output) on an individual line basis.

	RS registers relevant to peripheral ports:
	(a register is selected by setting an address to the 4-bit RS lines)
		0x00: ORB/IRB; write: Output Register B, read: Input Register "B".
		0x01: ORA/IRA; write: Output Register A, read: Input Register "A".
		0x02: DDRB; Data Direction Register B
		0x03: DDRA; Data Direction Register A
		0x0C: PCR; Peripheral Control Register.
		      0: CA1 control, 1..3: CA2 control
		      4: CB1 control, 5..7: CB2 control.

	External interface relevant to peripheral ports:
	PORTA: 8-bit independently bidirectional data to peripheral.
	PORTB: 8-bit independently bidirectional data to peripheral.
	DATA: 8-bit bidirectional data to microprocessor.
	RS: 4-bit register select.
	CA: 2-bit control lines for PORTA.
	CB: 2-bit control lines for PORTB.

	Write handshake control (PORT A as example, PORT B is same for writes):
	  CA2 (output) indicates data has been written to ORA and is ready.
	  CA1 (input) indicates data has been taken.
	Default modes assuming PCR == 0x00:
	  CA2: Input-negative active edge (one of eight options).
	  CA1: negative active edge (one of two options).

	Timers

	Timers have not yet been implemented.

	Interrupts

	Interrupts have not yet been implemented.

	Reference Material

	The following data sheets and external resources may be useful.

		Original 6522: http://en.wikipedia.org/wiki/MOS_Technology_6522
		WCD 65C22: http://www.westerndesigncenter.com/wdc/w65c22-chip.cfm
		Data sheet: http://www.westerndesigncenter.com/wdc/documentation/w65c22.pdf
*/
package via6522

import (
	"fmt"
	"strconv"
	"unicode"
)

const (
	viaOrb = 0x0
	viaIrb = 0x0
	viaOra = 0x1
	viaIra = 0x1

	viaDdrb = 0x2
	viaDdra = 0x3

	// bit-offset into PCR for port A & B
	viaPcrOffsetA = 0
	viaPcrOffsetB = 4
)

/**
 * Memory interface implementation.
 */

// The internal state of the 6522 VIA controller, and references to connected
// peripheral devices.
type Via6522 struct {
	// Note: It may be a mistake to consider ORx and IRx separate registers.
	//       If so... fix it?
	ora           byte // output register port A
	orb           byte // output register port B
	ira           byte // input register port A
	irb           byte // input register port B
	ddra          byte // data direction port A
	ddrb          byte // data direction port B
	pcr           byte // peripheral control register
	options       Options
	paPeripherals []ParallelPeripheral
	pbPeripherals []ParallelPeripheral
}

type Options struct {
	DumpBinary bool
	DumpAscii  bool
}

// ParallelPeripheral defines an interface for peripheral devices which can connect to
// either of the parallel ports to read and write data.
type ParallelPeripheral interface {

	// PinMask is a bitfield representing which VIA pins the device is
	// connected to. 1 = connected, 0 = not connected.
	PinMask() byte

	// Read returns the state of the device's output pins (VIA DDR input).
	// Bits not set in PinMask will be ignored.
	Read() byte

	// Shutdown runs tear-down tasks when the system is shutting down.
	Shutdown()

	// Write is passed the updated port state when data is written.
	// Bits not set in PinMask should be ignored.
	Write(byte)

	String() string
}

func NewVia6522(o Options) *Via6522 {
	via := &Via6522{}
	via.options = o
	via.paPeripherals = make([]ParallelPeripheral, 0)
	via.pbPeripherals = make([]ParallelPeripheral, 0)
	return via
}

// AttachToPortA attaches a ParallelPeripheral to PA.
func (via *Via6522) AttachToPortA(p ParallelPeripheral) {
	fmt.Printf("%s PORTA attaching %s (pinmask: %08b)\n", via, p, p.PinMask())
	via.paPeripherals = append(via.paPeripherals, p)
}

// AttachToPortA attaches a ParallelPeripheral to PB.
func (via *Via6522) AttachToPortB(p ParallelPeripheral) {
	fmt.Printf("%s PORTB attaching %s (pinmask: %08b)\n", via, p, p.PinMask())
	via.pbPeripherals = append(via.pbPeripherals, p)
}

// Shutdown tells Via6522 and its devices that the system is shutting down.
func (via *Via6522) Shutdown() {
	var p ParallelPeripheral
	for _, p = range via.paPeripherals {
		fmt.Printf("%s shutting down PORTA peripheral: %s\n", via, p.String())
		p.Shutdown()
	}
	for _, p = range via.pbPeripherals {
		fmt.Printf("%s shutting down PORTB peripheral: %s\n", via, p.String())
		p.Shutdown()
	}
}

// CA1 or CB1 1-bit mode for the given port offset (viaPCR_OFFSET_x)
func (via *Via6522) control1Mode(portOffset uint8) byte {
	return (via.pcr >> portOffset) & 1
}

// CA2 or CB2 3-bit mode for the given port offset (viaPCR_OFFSET_x)
func (via *Via6522) control2Mode(portOffset uint8) byte {
	return (via.pcr >> (portOffset + 1)) & 0x7
}

// Print a byte as ASCII, using escape sequences where necessary.
func printAsciiByte(b uint8) {
	r := rune(b)
	if unicode.IsPrint(r) || unicode.IsSpace(r) {
		fmt.Print(string(r))
	} else {
		charStr := strconv.QuoteRuneToASCII(r)
		fmt.Print(charStr[1 : len(charStr)-1])
	}
}

// Read the register specified by the given 4-bit address (0x00..0x0F).
// TODO: Unlike IRA, reading IRB actully returns bits from ORA for pins
//       that are programmed as output.
func (via *Via6522) Read(a uint16) byte {
	switch a {
	default:
		panic(fmt.Sprintf("read from 0x%X not handled by Via6522", a))
	case 0x0:
		via.irb = 0x00
		for _, p := range via.pbPeripherals {
			via.irb |= (p.Read() & p.PinMask())
		}
		return via.readMixedInputOutput(via.irb, via.orb, via.ddrb)
	case 0x1:
		via.ira = 0x00
		for _, p := range via.paPeripherals {
			via.ira |= (p.Read() & p.PinMask())
		}
		return via.readMixedInputOutput(via.ira, via.ora, via.ddra)
	case 0x2:
		return via.ddrb
	case 0x3:
		return via.ddra
	case 0xC:
		return via.pcr
	}
}

// This represents the correct behavior for reading IRB,
// and maybe an approximation of the correct behavior for IRA.
func (via *Via6522) readMixedInputOutput(in byte, out byte, ddr byte) byte {
	return (out & ddr) | (in & ^ddr)
}

// From the datasheet:
// Reset clears all internal registers
// (except T1 and T2 counters and latches, and the SR.)
func (via *Via6522) Reset() {
	via.ora = 0
	via.orb = 0
	via.ira = 0
	via.irb = 0
	via.ddra = 0
	via.ddrb = 0
	via.pcr = 0
}

// The address size of the memory-mapped IO.
// Helps to meet the go6502.Memory interface.
func (via *Via6522) Size() int {
	return 16 // 4-bit RS exposes 16 byte address space.
}

func (via *Via6522) String() string {
	return "VIA6522"
}

// Write to register specified by the given 4-bit address (0x00..0x0F).
func (via *Via6522) Write(a uint16, data byte) {
	switch a {
	default:
		panic(fmt.Sprintf("write to 0x%X not handled by Via6522", a))
	case 0x0:
		via.orb = data
		via.handleDataWrite(data&via.ddrb, via.pbPeripherals)
	case 0x1:
		via.ora = data
		via.handleDataWrite(data&via.ddra, via.paPeripherals)
	case 0x2:
		via.ddrb = data
	case 0x3:
		via.ddra = data
	case 0xC:
		via.pcr = data
	}
}

func (via *Via6522) handleDataWrite(data byte, peripherals []ParallelPeripheral) {
	if via.options.DumpBinary {
		fmt.Printf("VIA output: %08b (0x%02X)\n", data, data)
	}
	if via.options.DumpAscii {
		printAsciiByte(data)
	}
	for _, p := range peripherals {
		p.Write(data & p.PinMask())
	}
}
