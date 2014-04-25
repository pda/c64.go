package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/pda/go6502/go6502"
)

const (
	kernalPath = "rom/kernal.rom"
)

func main() {
	os.Exit(mainReturningStatus())
}

func mainReturningStatus() int {

	options := go6502.ParseOptions()

	kernal, err := go6502.RomFromFile(kernalPath)
	if err != nil {
		panic(err)
	}

	ram := &go6502.Ram{}

	via := go6502.NewVia6522(options)
	if options.ViaSsd1306 {
		ssd1306 := go6502.NewSsd1306()
		defer ssd1306.Close()
		via.AttachToPortB(ssd1306)
	}

	via.Reset()

	addressBus, _ := go6502.CreateBus()
	addressBus.Attach(ram, "ram", 0x0000)
	addressBus.Attach(via, "VIA", 0xC000)
	addressBus.Attach(kernal, "kernal", 0xE000)

	exitChan := make(chan int, 0)

	cpu := &go6502.Cpu{Bus: addressBus, ExitChan: exitChan}
	if options.Debug {
		debugger := go6502.NewDebugger(cpu)
		defer debugger.Close()
		debugger.QueueCommands(options.DebugCmds)
		cpu.AttachDebugger(debugger)
	}
	cpu.Reset()

	// Dispatch CPU in a goroutine.
	go func() {
		i := 0
		for {
			cpu.Step()
			i++
		}
	}()

	var (
		sig        os.Signal
		exitStatus int
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	select {
	case exitStatus = <-exitChan:
		// pass
	case sig = <-sigChan:
		fmt.Println("\nGot signal:", sig)
		exitStatus = 1
	}

	if exitStatus != 0 {
		fmt.Println(cpu)
		fmt.Println("Dumping RAM into core file")
		ram.Dump("core")
	}

	return exitStatus
}