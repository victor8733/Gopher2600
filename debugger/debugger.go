package debugger

import (
	"fmt"
	"gopher2600/hardware"
	"gopher2600/hardware/cpu"
	"gopher2600/television"
	"os"
	"os/signal"
	"strings"
)

// Debugger is the basic debugging frontend for the emulation
type Debugger struct {
	vcs     *hardware.VCS
	running bool
	input   []byte

	breakpoints   *breakpoints
	runUntilBreak bool

	// commandOnHalt says whether an sequence of commands should run automatically
	// when emulation halts. commandOnHaltPrev is the stored command sequence
	// used when ONHALT is called with no arguments
	commandOnHalt       string
	commandOnHaltStored string

	// verbose controls the verbosity of commands that echo machine state
	verbose bool

	// input loop fields. we're storing these here because inputLoop can be
	// called from within another input loop (via a video step callback) and we
	// want these properties to persist
	inputloopBreakpoint bool
	inputloopNext       bool
	inputloopVideoStep  bool

	/* user interface */

	// UserPrint is a definable function for printing to the screen. ie. the
	// function that presents the string to the user. note that the debugger
	// calls UserPrint via the dbg.print function
	UserPrint func(PrintProfile, string, ...interface{})

	// UserRead gets the next line of input from the user
	UserRead func([]byte) (int, error)
}

// NewDebugger is the preferred method of initialisation for the Debugger structure
func NewDebugger() (*Debugger, error) {
	var err error

	dbg := new(Debugger)

	// prepare hardware
	tv, err := television.NewSDLTV("NTSC", 3)
	if err != nil {
		return nil, err
	}
	dbg.vcs, err = hardware.New(tv)
	if err != nil {
		return nil, err
	}

	// allocate memory for user input
	dbg.input = make([]byte, 255)

	// set up breakpoints
	dbg.breakpoints = newBreakpoints()

	// default ONHALT command squence
	dbg.commandOnHaltStored = "CPU; TIA; TV"

	// default commands to user interface callbacks
	dbg.UserPrint = plainPrint
	dbg.UserRead = plainRead

	return dbg, nil
}

// Start the main debugger sequence
func (dbg *Debugger) Start(filename string) error {
	err := dbg.vcs.AttachCartridge(filename)
	if err != nil {
		return err
	}

	// register ctrl-c handler
	ctrlC := make(chan os.Signal)
	signal.Notify(ctrlC, os.Interrupt)
	go func() {
		for dbg.running {
			<-ctrlC
			if dbg.runUntilBreak == true {
				dbg.runUntilBreak = false
			} else {
				// TODO: interrupt os.Stdin.Read()
				dbg.running = false
			}
		}
	}()

	// prepare and run main input loop
	dbg.inputloopBreakpoint = true
	dbg.inputloopNext = true
	dbg.running = true
	err = dbg.inputLoop(true)
	if err != nil {
		return err
	}
	return nil
}

// videoCycleInputLoop is a wrapper function to be used when calling vcs.Step()
func (dbg *Debugger) videoCycleInputLoop() error {
	return dbg.inputLoop(false)
}

// inputLoop has two modes, defined by the mainLoop argument. a value of false
// (ie not a mainLoop) cases the function to return in those situations when
// the main loop (value of true) would carry on. a mainLoop of false helps us
// to implement video stepping.
func (dbg *Debugger) inputLoop(mainLoop bool) error {
	var err error
	var result *cpu.InstructionResult

	for dbg.running {
		// return immediately if we're in a mid-cycle input loop and we don't want
		// to be
		if !mainLoop && !dbg.inputloopVideoStep {
			return nil
		}

		// check for breakpoint. breakpoint check echos the break condition if it
		// matches
		dbg.inputloopBreakpoint = (dbg.inputloopNext && dbg.breakpoints.check(dbg, result))

		// if haltCommand mode and if run state is correct that print haltCommand
		// command(s)
		if dbg.commandOnHalt != "" {
			if (dbg.inputloopNext && !dbg.runUntilBreak) || dbg.inputloopBreakpoint {
				_, _ = dbg.parseInput(dbg.commandOnHalt)
			}
		}

		// expand breakpoint to include step-once/many flag
		dbg.inputloopBreakpoint = dbg.inputloopBreakpoint || !dbg.runUntilBreak

		if dbg.inputloopBreakpoint {
			// force update of tv image on break
			err = dbg.vcs.TV.ForceUpdate()
			if err != nil {
				return err
			}

			// reset run until break condition
			dbg.runUntilBreak = false

			// get user input
			dbg.print(Prompt, "[0x%04x] > ", dbg.vcs.MC.PC.ToUint16())
			n, err := dbg.UserRead(dbg.input)
			if err != nil {
				return err
			}

			// parse user input
			dbg.inputloopNext, err = dbg.parseInput(string(dbg.input[:n-1]))
			if err != nil {
				dbg.print(Error, "%s\n", err)
			}

			// prepare for next loop
			dbg.inputloopBreakpoint = false
		}

		// move emulation on one step if user has requested/implied it
		if dbg.inputloopNext {
			if mainLoop {
				_, result, err = dbg.vcs.Step(dbg.videoCycleInputLoop)
				if err != nil {
					return err
				}
				dbg.print(StepResult, "%v\n", result)
			} else {
				return nil
			}
		}
	}

	return nil
}

// parseInput splits the input into individual commands. each command is then
// passed to parseCommand for final processing
func (dbg *Debugger) parseInput(input string) (bool, error) {
	var cont bool
	var err error

	commands := strings.Split(strings.ToUpper(input), ";")
	for i := 0; i < len(commands); i++ {
		cont, err = dbg.parseCommand(commands[i])
		if err != nil {
			return false, err
		}
	}

	return cont, nil
}

// parseCommand scans user input for valid commands and acts upon it. commands
// that cause the emulation to move forward (RUN, STEP) return true for the
// first return value. other commands return false and act upon the command
// immediately. note that the empty string is the same as the STEP command
func (dbg *Debugger) parseCommand(input string) (bool, error) {

	// remove leading/trailing space
	input = strings.TrimSpace(input)

	// if the input is empty then return true, indicating that the emulation
	// should "step" forward once
	if input == "" {
		return true, nil
	}

	// divide user input into parts and convert to upper-case for easy parsing
	// input is unchanged in case we need the original user-case
	parts := strings.Split(strings.ToUpper(input), " ")

	// Go's strings.Split() command appends an empty string for every additional
	// space in the input. the for-loop is a little post processing to sanitise
	// the parts array.
	// TODO: perhaps it would be better to write our own Split() function
	partsb := make([]string, 0)
	for i := 0; i < len(parts); i++ {
		if parts[i] != "" {
			partsb = append(partsb, parts[i])
		}
	}
	parts = partsb

	// normalise variations in syntax
	for i := 0; i < len(parts); i++ {
		// normalise hex notation
		if parts[i][0] == '$' {
			parts[i] = fmt.Sprintf("0x%s", parts[i][1:])
		}
	}

	// most commands do not cause the emulator to step forward
	stepNext := false

	// first entry in parts is the debugging command. switch on this value
	switch parts[0] {
	default:
		return false, fmt.Errorf("%s is not a debugging command", parts[0])

	// control of the debugger

	case "BREAK":
		err := dbg.breakpoints.parseBreakpoint(dbg, parts)
		if err != nil {
			return false, err
		}

	case "CLEAR":
		if len(parts) < 2 {
			return false, fmt.Errorf("not enough arguments for %s command", parts[0])
		}
		switch parts[1] {
		default:
			return false, fmt.Errorf("%s is not a valid %s command", parts[1], parts[0])
		case "BREAKS":
			dbg.breakpoints.clear()
			dbg.print(Feedback, "breakpoints cleared\n")
		}

	case "ONHALT":
		if len(parts) < 2 {
			dbg.commandOnHalt = dbg.commandOnHaltStored
		} else {
			if parts[1] == "OFF" {
				dbg.commandOnHalt = ""
				dbg.print(Feedback, "no auto-command on halt\n")
				return false, nil
			}

			// TODO: implement syntax checking when specifying ONHALT commands before
			// committing to the new sequnce

			// use remaininder of command line to form the ONHALt command sequence
			dbg.commandOnHalt = strings.Join(parts[1:], " ")

			// we can't use semi-colons when specifying the sequence so allow use of
			// commas to act as an alternative
			dbg.commandOnHalt = strings.Replace(dbg.commandOnHalt, ",", ";", -1)

			// store the new command so we can reuse it
			dbg.commandOnHaltStored = dbg.commandOnHalt
		}

		dbg.print(Feedback, "auto-command on halt: %s\n", dbg.commandOnHalt)

	case "MEMMAP":
		dbg.print(MachineInfo, "%v", dbg.vcs.Mem.MemoryMap())

	case "QUIT":
		dbg.running = false

	case "RESET":
		dbg.print(Feedback, "machine reset\n")
		err := dbg.vcs.Reset()
		if err != nil {
			return false, err
		}

	case "RUN":
		dbg.runUntilBreak = true
		stepNext = true

	case "STEP":
		stepNext = true
		if len(parts) > 1 {
			switch parts[1] {
			case "CPU":
				dbg.inputloopVideoStep = false
			case "VIDEO":
				dbg.inputloopVideoStep = true
			}
		}

	case "TERSE":
		dbg.verbose = false
		dbg.print(Feedback, "verbosity: terse\n")

	case "VERBOSE":
		dbg.verbose = true
		dbg.print(Feedback, "verbosity: verbose\n")

	// information about the machine (chips)

	case "CPU":
		dbg.printMachineInfo(dbg.vcs.MC)

	case "TIA":
		dbg.printMachineInfo(dbg.vcs.TIA)

	case "TV":
		dbg.printMachineInfo(dbg.vcs.TV)

	// information about the machine (sprites)

	case "BALL":
		dbg.printMachineInfo(dbg.vcs.TIA.Video.Ball)

	// tv control

	case "SHOW":
		err := dbg.vcs.TV.SetVisibility(true)
		if err != nil {
			return false, err
		}

	case "HIDE":
		err := dbg.vcs.TV.SetVisibility(false)
		if err != nil {
			return false, err
		}
	}

	return stepNext, nil
}
