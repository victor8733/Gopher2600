// This file is part of Gopher2600.
//
// Gopher2600 is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Gopher2600 is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Gopher2600.  If not, see <https://www.gnu.org/licenses/>.

package debugger

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jetsetilly/gopher2600/cartridgeloader"
	"github.com/jetsetilly/gopher2600/debugger/script"
	"github.com/jetsetilly/gopher2600/debugger/terminal"
	"github.com/jetsetilly/gopher2600/debugger/terminal/commandline"
	"github.com/jetsetilly/gopher2600/disassembly"
	"github.com/jetsetilly/gopher2600/errors"
	"github.com/jetsetilly/gopher2600/gui"
	"github.com/jetsetilly/gopher2600/hardware/cpu/registers"
	"github.com/jetsetilly/gopher2600/hardware/memory/memorymap"
	"github.com/jetsetilly/gopher2600/hardware/riot/input"
	"github.com/jetsetilly/gopher2600/linter"
	"github.com/jetsetilly/gopher2600/logger"
	"github.com/jetsetilly/gopher2600/patch"
	"github.com/jetsetilly/gopher2600/symbols"
)

var debuggerCommands *commandline.Commands
var scriptUnsafeCommands *commandline.Commands

// this init() function "compiles" the commandTemplate above into a more
// usuable form. It will cause the program to fail if the template is invalid.
func init() {
	var err error

	// parse command template
	debuggerCommands, err = commandline.ParseCommandTemplate(commandTemplate)
	if err != nil {
		panic(err)
	}

	err = debuggerCommands.AddHelp(cmdHelp, helps)
	if err != nil {
		panic(err)
	}
	sort.Stable(debuggerCommands)

	scriptUnsafeCommands, err = commandline.ParseCommandTemplate(scriptUnsafeTemplate)
	if err != nil {
		panic(err)
	}
	sort.Stable(scriptUnsafeCommands)
}

// parseCommand tokenises the input and processes the tokens
func (dbg *Debugger) parseCommand(cmd string, scribe bool, echo bool) (bool, error) {
	tokens, err := dbg.tokeniseCommand(cmd, scribe, echo)
	if err != nil {
		return false, err
	}
	return dbg.processTokens(tokens)
}

func (dbg *Debugger) tokeniseCommand(cmd string, scribe bool, echo bool) (*commandline.Tokens, error) {
	// tokenise input
	tokens := commandline.TokeniseInput(cmd)

	// if there are no tokens in the input then continue with onEmptyInput
	if tokens.Remaining() == 0 {
		return dbg.tokeniseCommand(onEmptyInput, true, false)
	}

	// check validity of tokenised input
	err := debuggerCommands.ValidateTokens(tokens)
	if err != nil {
		return nil, err
	}

	// print normalised input if this is command from an interactive source
	// and not an auto-command
	if echo {
		dbg.printLine(terminal.StyleEcho, tokens.String())
	}

	// test to see if command is allowed when recording/playing a script
	if dbg.scriptScribe.IsActive() && scribe {
		tokens.Reset()

		err := scriptUnsafeCommands.ValidateTokens(tokens)

		// fail when the tokens DO match the scriptUnsafe template (ie. when
		// there is no err from the validate function)
		if err == nil {
			return nil, errors.New(errors.CommandError, fmt.Sprintf("'%s' is unsafe to use in scripts", tokens.String()))
		}

		// record command if it auto is false (is not a result of an "auto" command
		// eg. ONHALT). if there's an error then the script will be rolled back and
		// the write removed.
		dbg.scriptScribe.WriteInput(tokens.String())
	}

	return tokens, nil
}

// processTokenGroup call processTokens for each entry in the array of tokens
func (dbg *Debugger) processTokenGroup(tokenGrp []*commandline.Tokens) (bool, error) {
	var err error
	var ok bool

	for _, t := range tokenGrp {
		ok, err = dbg.processTokens(t)
		if err != nil {
			return false, err
		}
	}
	return ok, nil
}

func (dbg *Debugger) processTokens(tokens *commandline.Tokens) (bool, error) {
	// check first token. if this token makes sense then we will consume the
	// rest of the tokens appropriately
	tokens.Reset()
	command, _ := tokens.Get()

	switch command {
	default:
		return false, errors.New(errors.CommandError, fmt.Sprintf("%s is not yet implemented", command))

	case cmdHelp:
		keyword, ok := tokens.Get()
		if ok {
			dbg.printLine(terminal.StyleHelp, debuggerCommands.Help(keyword))
		} else {
			dbg.printLine(terminal.StyleHelp, debuggerCommands.HelpOverview())
		}

		// help can be called during script recording but we don't want to
		// include it
		dbg.scriptScribe.Rollback()

		return false, nil

	case cmdQuit:
		if dbg.scriptScribe.IsActive() {
			dbg.printLine(terminal.StyleFeedback, "ending script recording")

			// QUIT when script is being recorded is the same as SCRIPT END
			//
			// we don't want the QUIT command to appear in the script so
			// rollback last entry before we commit it in EndSession()
			dbg.scriptScribe.Rollback()
			dbg.scriptScribe.EndSession()
		} else {
			dbg.running = false
		}

	case cmdReset:
		err := dbg.VCS.Reset()
		if err != nil {
			return false, err
		}
		dbg.printLine(terminal.StyleFeedback, "machine reset")

	case cmdRun:
		dbg.runUntilHalt = true
		return true, nil

	case cmdHalt:
		dbg.haltImmediately = true

	case cmdStep:
		mode, _ := tokens.Get()
		mode = strings.ToUpper(mode)
		switch mode {
		case "":
			// calling step with no argument is the normal case
		case "CPU":
			// changes quantum
			dbg.quantum = QuantumCPU
		case "VIDEO":
			// changes quantum
			dbg.quantum = QuantumVideo
		default:
			// does not change quantum
			tokens.Unget()
			err := dbg.stepTraps.parseCommand(tokens)
			if err != nil {
				return false, errors.New(errors.CommandError, fmt.Sprintf("unknown step mode (%s)", mode))
			}
			dbg.runUntilHalt = true
		}

		return true, nil

	case cmdQuantum:
		mode, _ := tokens.Get()
		mode = strings.ToUpper(mode)
		switch mode {
		case "CPU":
			dbg.quantum = QuantumCPU
		case "VIDEO":
			dbg.quantum = QuantumVideo
		default:
			dbg.printLine(terminal.StyleFeedback, "set to %s", dbg.quantum)
		}

	case cmdScript:
		option, _ := tokens.Get()
		switch strings.ToUpper(option) {
		case "RECORD":
			var err error
			saveFile, _ := tokens.Get()
			err = dbg.scriptScribe.StartSession(saveFile)
			if err != nil {
				return false, err
			}

			// we don't want SCRIPT RECORD command to appear in the
			// script
			dbg.scriptScribe.Rollback()

			return false, nil

		case "END":
			dbg.scriptScribe.Rollback()
			err := dbg.scriptScribe.EndSession()
			return false, err

		default:
			// run a script
			scr, err := script.RescribeScript(option)
			if err != nil {
				return false, err
			}

			if dbg.scriptScribe.IsActive() {
				// if we're currently recording a script we want to write this
				// command to the new script file but indicate that we'll be
				// entering a new script and so don't want to repeat the
				// commands from that script
				dbg.scriptScribe.StartPlayback()

				defer func() {
					dbg.scriptScribe.EndPlayback()
				}()
			}

			err = dbg.inputLoop(scr, false)
			if err != nil {
				return false, err
			}
		}

	case cmdInsert:
		cart, _ := tokens.Get()
		err := dbg.loadCartridge(cartridgeloader.NewLoader(cart, "AUTO"))
		if err != nil {
			return false, err
		}
		dbg.printLine(terminal.StyleFeedback, "machine reset with new cartridge (%s)", cart)

	case cmdCartridge:
		arg, ok := tokens.Get()
		if ok {
			switch arg {
			case "BANK":
				dbg.printLine(
					terminal.StyleInstrument,
					fmt.Sprintf("%s", dbg.VCS.Mem.Cart.MappingSummary()),
				)

			case "STATIC":
				// !!TODO: poke/peek static cartridge static data areas
				if bus := dbg.VCS.Mem.Cart.GetStaticBus(); bus != nil {
					s := &strings.Builder{}
					static := bus.GetStatic()
					if static != nil {
						for b := 0; b < len(static); b++ {
							s.WriteString(static[b].Label + "\n")

							// header for table. assumes that origin address begins at xxx0
							s.WriteString("        -0 -1 -2 -3 -4 -5 -6 -7 -8 -9 -A -B -C -D -E -F\n")
							s.WriteString("      ---- -- -- -- -- -- -- -- -- -- -- -- -- -- -- --")

							for i := 0; i < len(static[b].Data); i++ {
								// begin new row every 16 iterations
								if i%16 == 0 {
									s.WriteString(fmt.Sprintf("\n%03x- |  ", i/16))
								}
								d, _ := dbg.VCS.Mem.Read(uint16(i))
								s.WriteString(fmt.Sprintf("%02x ", d))
							}
							s.WriteString("\n\n")
						}

						dbg.printLine(terminal.StyleInstrument, s.String())
					} else {
						dbg.printLine(terminal.StyleFeedback, "cartridge has no static data areas")
					}
				} else {
					dbg.printLine(terminal.StyleFeedback, "cartridge has no static data areas")
				}
			case "REGISTERS":
				// !!TODO: poke/peek cartridge registers
				if bus := dbg.VCS.Mem.Cart.GetRegistersBus(); bus != nil {
					dbg.printLine(terminal.StyleInstrument, bus.GetRegisters().String())
				} else {
					dbg.printLine(terminal.StyleFeedback, "cartridge has no registers")
				}

			case "RAM":
				// cartridge RAM is accessible through the normal VCS buses so
				// the normal peek/poke commands will work
				if bus := dbg.VCS.Mem.Cart.GetRAMbus(); bus != nil {
					s := &strings.Builder{}
					ram := bus.GetRAM()
					if ram != nil {
						for b := 0; b < len(ram); b++ {
							s.WriteString(ram[b].Label + "\n")

							// header for table. assumes that origin address begins at xxx0
							s.WriteString("        -0 -1 -2 -3 -4 -5 -6 -7 -8 -9 -A -B -C -D -E -F\n")
							s.WriteString("      ---- -- -- -- -- -- -- -- -- -- -- -- -- -- -- --")

							for i := 0; i < len(ram[b].Data); i++ {
								// begin new row every 16 iterations
								if i%16 == 0 {
									s.WriteString(fmt.Sprintf("\n%03x- |  ", i/16))
								}
								d, _ := dbg.VCS.Mem.Read(ram[b].Origin + uint16(i))
								s.WriteString(fmt.Sprintf("%02x ", d))
							}
							s.WriteString("\n\n")
						}

						dbg.printLine(terminal.StyleInstrument, s.String())
					} else {
						dbg.printLine(terminal.StyleFeedback, "cartridge has no RAM")
					}
				} else {
					dbg.printLine(terminal.StyleFeedback, "cartridge has no RAM")
				}
			}
		} else {
			dbg.printLine(terminal.StyleInstrument, dbg.VCS.Mem.Cart.String())
		}

	case cmdPatch:
		f, _ := tokens.Get()
		patched, err := patch.CartridgeMemory(dbg.VCS.Mem.Cart, f)
		if err != nil {
			dbg.printLine(terminal.StyleError, "%v", err)
			if patched {
				dbg.printLine(terminal.StyleError, "error during patching. cartridge might be unusable.")
			}
			return false, nil
		}
		if patched {
			dbg.printLine(terminal.StyleFeedback, "cartridge patched")
		}

	case cmdDisassembly:
		bytecode := false
		bank := -1

		arg, ok := tokens.Get()
		if ok {
			switch arg {
			case "BYTECODE":
				bytecode = true
			default:
				bank, _ = strconv.Atoi(arg)
			}
		}

		var err error

		attr := disassembly.WriteAttr{ByteCode: bytecode}
		s := &bytes.Buffer{}

		if bank == -1 {
			err = dbg.Disasm.Write(s, attr)
		} else {
			err = dbg.Disasm.WriteBank(s, attr, bank)
		}

		if err != nil {
			return false, err
		}

		dbg.printLine(terminal.StyleFeedback, s.String())

	case cmdLint:
		output := &strings.Builder{}
		err := linter.Lint(dbg.Disasm, output)
		if err != nil {
			return false, err
		}
		dbg.printLine(terminal.StyleFeedback, output.String())

	case cmdGrep:
		scope := disassembly.GrepAll

		s, _ := tokens.Get()
		switch strings.ToUpper(s) {
		case "MNEMONIC":
			scope = disassembly.GrepMnemonic
		case "OPERAND":
			scope = disassembly.GrepOperand
		default:
			tokens.Unget()
		}

		search, _ := tokens.Get()
		output := &strings.Builder{}
		err := dbg.Disasm.Grep(output, scope, search, false)
		if err != nil {
			return false, nil
		}
		if output.Len() == 0 {
			dbg.printLine(terminal.StyleError, "%s not found in disassembly", search)
		} else {
			dbg.printLine(terminal.StyleFeedback, output.String())
		}

	case cmdSymbol:
		tok, _ := tokens.Get()
		switch strings.ToUpper(tok) {
		case "LIST":
			option, ok := tokens.Get()
			if ok {
				switch strings.ToUpper(option) {
				default:
					// already caught by command line ValidateTokens()

				case "LOCATIONS":
					dbg.Disasm.Symtable.ListLocations(dbg.printStyle(terminal.StyleFeedback))

				case "READ":
					dbg.Disasm.Symtable.ListReadSymbols(dbg.printStyle(terminal.StyleFeedback))

				case "WRITE":
					dbg.Disasm.Symtable.ListWriteSymbols(dbg.printStyle(terminal.StyleFeedback))
				}
			} else {
				dbg.Disasm.Symtable.ListSymbols(dbg.printStyle(terminal.StyleFeedback))
			}

		default:
			symbol := tok
			table, symbol, address, err := dbg.Disasm.Symtable.SearchSymbol(symbol, symbols.UnspecifiedSymTable)
			if err != nil {
				if errors.Is(err, errors.SymbolUnknown) {
					dbg.printLine(terminal.StyleFeedback, "%s -> not found", symbol)
					return false, nil
				}
				return false, err
			}

			option, ok := tokens.Get()
			if ok {
				switch strings.ToUpper(option) {
				default:
					// already caught by command line ValidateTokens()

				case "ALL", "MIRRORS":
					dbg.printLine(terminal.StyleFeedback, "%s -> %#04x", symbol, address)

					// find all instances of symbol address in memory space
					// assumption: the address returned by SearchSymbol is the
					// first address in the complete list
					for m := address + 1; m < memorymap.OriginCart; m++ {
						ai := dbg.dbgmem.mapAddress(m, table == symbols.ReadSymTable)
						if ai.mappedAddress == address {
							dbg.printLine(terminal.StyleFeedback, "%s (%s) -> %#04x", symbol, table, m)
						}
					}
				}
			} else {
				dbg.printLine(terminal.StyleFeedback, "%s (%s) -> %#04x", symbol, table, address)
			}
		}

	case cmdOnHalt:
		if tokens.Remaining() == 0 {
			if len(dbg.commandOnHalt) == 0 {
				dbg.printLine(terminal.StyleFeedback, "auto-command on halt: OFF")
			} else {
				s := strings.Builder{}
				for _, c := range dbg.commandOnHalt {
					s.WriteString(c.String())
					s.WriteString("; ")
				}
				dbg.printLine(terminal.StyleFeedback, "command on halt: %s", strings.TrimSuffix(s.String(), "; "))
			}
			return false, nil
		}

		var input string

		option, _ := tokens.Get()
		switch strings.ToUpper(option) {
		case "OFF":
			dbg.commandOnHalt = dbg.commandOnHalt[:0]
			dbg.printLine(terminal.StyleFeedback, "no command on halt")
			return false, nil

		case "ON":
			dbg.commandOnHalt = dbg.commandOnHaltStored
			for _, c := range dbg.commandOnHalt {
				dbg.printLine(terminal.StyleFeedback, "auto-command on halt: %s", c)
			}
			return false, nil

		default:
			// token isn't one we recognise so push it back onto the token queue
			tokens.Unget()

			// use remaininder of command line to form the ONHALT command sequence
			input = strings.TrimSpace(tokens.Remainder())
			tokens.End()
		}

		// empty list of tokens. taking note of existing command - not the same
		// as commandOnHaltStored because ONHALT might be OFF
		existingOnHalt := dbg.commandOnHalt
		dbg.commandOnHalt = dbg.commandOnHalt[:0]

		// tokenise commands to check for integrity
		for _, s := range strings.Split(input, ",") {
			toks, err := dbg.tokeniseCommand(s, false, false)
			if err != nil {
				dbg.commandOnHalt = existingOnHalt
				return false, err
			}
			dbg.commandOnHalt = append(dbg.commandOnHalt, toks)
		}

		// make a copy of
		dbg.commandOnHaltStored = dbg.commandOnHalt

		// display the new ONHALT command(s)
		s := strings.Builder{}
		for _, c := range dbg.commandOnHalt {
			s.WriteString(c.String())
			s.WriteString("; ")
		}
		dbg.printLine(terminal.StyleFeedback, "command on halt: %s", strings.TrimSuffix(s.String(), "; "))

		return false, nil

	case cmdOnStep:
		if tokens.Remaining() == 0 {
			if len(dbg.commandOnStep) == 0 {
				dbg.printLine(terminal.StyleFeedback, "no command on step")
			} else {
				s := strings.Builder{}
				for _, c := range dbg.commandOnStep {
					s.WriteString(c.String())
					s.WriteString("; ")
				}
				dbg.printLine(terminal.StyleFeedback, "command on step: %s", strings.TrimSuffix(s.String(), "; "))
			}
			return false, nil
		}

		var input string

		option, _ := tokens.Get()
		switch strings.ToUpper(option) {
		case "OFF":
			dbg.commandOnStep = dbg.commandOnStep[:0]
			dbg.printLine(terminal.StyleFeedback, "auto-command on step: OFF")
			return false, nil

		case "ON":
			dbg.commandOnStep = dbg.commandOnStepStored
			for _, c := range dbg.commandOnStep {
				dbg.printLine(terminal.StyleFeedback, "auto-command on step: %s", c)
			}
			return false, nil

		default:
			// token isn't one we recognise so push it back onto the token queue
			tokens.Unget()

			// use remaininder of command line to form the ONSTEP command sequence
			input = strings.TrimSpace(tokens.Remainder())
			tokens.End()
		}

		// empty list of tokens. taking note of existing command - not the same
		// as commandOnStepStored because ONSTEP might be OFF
		existingOnStep := dbg.commandOnStep
		dbg.commandOnStep = dbg.commandOnStep[:0]

		// tokenise commands to check for integrity
		for _, s := range strings.Split(input, ",") {
			toks, err := dbg.tokeniseCommand(s, false, false)
			if err != nil {
				dbg.commandOnStep = existingOnStep
				return false, err
			}
			dbg.commandOnStep = append(dbg.commandOnStep, toks)
		}

		// store new commandOnStep
		dbg.commandOnStepStored = dbg.commandOnStep

		// display the new ONSTEP command(s)
		s := strings.Builder{}
		for _, c := range dbg.commandOnStep {
			s.WriteString(c.String())
			s.WriteString("; ")
		}
		dbg.printLine(terminal.StyleFeedback, "command on step: %s", strings.TrimSuffix(s.String(), "; "))

		return false, nil

	case cmdOnTrace:
		if tokens.Remaining() == 0 {
			if len(dbg.commandOnTrace) == 0 {
				dbg.printLine(terminal.StyleFeedback, "no command on trace")
			} else {
				s := strings.Builder{}
				for _, c := range dbg.commandOnTrace {
					s.WriteString(c.String())
					s.WriteString("; ")
				}
				dbg.printLine(terminal.StyleFeedback, "command on trace: %s", strings.TrimSuffix(s.String(), "; "))
			}
			return false, nil
		}

		var input string

		option, _ := tokens.Get()
		switch strings.ToUpper(option) {
		case "OFF":
			dbg.commandOnTrace = dbg.commandOnTrace[:0]
			dbg.printLine(terminal.StyleFeedback, "auto-command on trace: OFF")
			return false, nil

		case "ON":
			dbg.commandOnTrace = dbg.commandOnTraceStored
			for _, c := range dbg.commandOnTrace {
				dbg.printLine(terminal.StyleFeedback, "auto-command on trace: %s", c)
			}
			return false, nil

		default:
			// token isn't one we recognise so push it back onto the token queue
			tokens.Unget()

			// use remaininder of command line to form the ONTRACE command sequence
			input = strings.TrimSpace(tokens.Remainder())
			tokens.End()
		}

		// empty list of tokens. taking note of existing command
		existingOnTrace := dbg.commandOnTrace
		dbg.commandOnTrace = dbg.commandOnTrace[:0]

		// tokenise commands to check for integrity
		for _, s := range strings.Split(input, ",") {
			toks, err := dbg.tokeniseCommand(s, false, false)
			if err != nil {
				dbg.commandOnTrace = existingOnTrace
				return false, err
			}
			dbg.commandOnTrace = append(dbg.commandOnTrace, toks)
			fmt.Println(toks)
		}

		// store new commandOnTrace
		dbg.commandOnTraceStored = dbg.commandOnTrace

		// display the new ONTRACE command(s)
		s := strings.Builder{}
		for _, c := range dbg.commandOnTrace {
			s.WriteString(c.String())
			s.WriteString("; ")
		}
		dbg.printLine(terminal.StyleFeedback, "command on trace: %s", strings.TrimSuffix(s.String(), "; "))

		return false, nil

	case cmdLast:
		if dbg.lastResult == nil || dbg.lastResult.Result.Defn == nil {
			dbg.printLine(terminal.StyleFeedback, "no instruction decoded yet")
			return false, nil
		}

		// whether to show bytecode
		bytecode := false

		option, ok := tokens.Get()
		if ok {
			switch strings.ToUpper(option) {
			case "DEFN":
				if dbg.VCS.CPU.LastResult.Defn == nil {
					dbg.printLine(terminal.StyleFeedback, "no instruction decoded yet")
				} else {
					dbg.printLine(terminal.StyleFeedback, "%s", dbg.VCS.CPU.LastResult.Defn)
				}
				return false, nil

			case "BYTECODE":
				bytecode = true
			}
		}

		s := strings.Builder{}

		if dbg.VCS.Mem.Cart.NumBanks() > 1 {
			s.WriteString(fmt.Sprintf("[%s] ", dbg.lastResult.Bank))
		}
		s.WriteString(dbg.Disasm.GetField(disassembly.FldAddress, dbg.lastResult))
		s.WriteString(" ")
		if bytecode {
			s.WriteString(dbg.Disasm.GetField(disassembly.FldBytecode, dbg.lastResult))
			s.WriteString(" ")
		}
		s.WriteString(dbg.Disasm.GetField(disassembly.FldMnemonic, dbg.lastResult))
		s.WriteString(" ")
		s.WriteString(dbg.Disasm.GetField(disassembly.FldOperand, dbg.lastResult))
		s.WriteString(" ")
		s.WriteString(dbg.Disasm.GetField(disassembly.FldActualCycles, dbg.lastResult))
		s.WriteString(" ")
		if !dbg.lastResult.Result.Final {
			s.WriteString(fmt.Sprintf("(of %d) ", dbg.lastResult.Result.Defn.Cycles))
		}
		s.WriteString(dbg.Disasm.GetField(disassembly.FldActualNotes, dbg.lastResult))

		// change terminal output style depending on condition of last CPU result
		if dbg.lastResult.Result.Final {
			dbg.printLine(terminal.StyleCPUStep, s.String())
		} else {
			dbg.printLine(terminal.StyleVideoStep, s.String())
		}

	case cmdMemMap:
		address, ok := tokens.Get()
		if ok {

			// if an address argument has been specified then map the address
			// in a read and write context and display the information

			// if hasMapped is false after the read/write mappings then the
			// address could no be resolved and we print an appropriate notice
			// to the user
			hasMapped := false

			s := strings.Builder{}

			ai := dbg.dbgmem.mapAddress(address, true)
			if ai != nil {
				hasMapped = true
				s.WriteString("Read:\n")
				if ai.address != ai.mappedAddress {
					s.WriteString(fmt.Sprintf("  %#04x maps to %#04x ", ai.address, ai.mappedAddress))
				} else {
					s.WriteString(fmt.Sprintf("  %#04x ", ai.address))
				}
				s.WriteString(fmt.Sprintf("in area %s\n", ai.area.String()))
				if ai.addressLabel != "" {
					s.WriteString(fmt.Sprintf("  labelled as %s\n", ai.addressLabel))
				}
			}
			ai = dbg.dbgmem.mapAddress(address, false)
			if ai != nil {
				hasMapped = true
				s.WriteString("Write:\n")
				if ai.address != ai.mappedAddress {
					s.WriteString(fmt.Sprintf("  %#04x maps to %#04x ", ai.address, ai.mappedAddress))
				} else {
					s.WriteString(fmt.Sprintf("  %#04x ", ai.address))
				}
				s.WriteString(fmt.Sprintf("in area %s\n", ai.area.String()))
				if ai.addressLabel != "" {
					s.WriteString(fmt.Sprintf("  labelled as %s\n", ai.addressLabel))
				}
			}

			// print results
			if hasMapped {
				dbg.printLine(terminal.StyleInstrument, "%s", s.String())
			} else {
				dbg.printLine(terminal.StyleFeedback, fmt.Sprintf("%v is not a mappable address", address))
			}

		} else {
			// without an address argument print the memorymap summary table
			dbg.printLine(terminal.StyleInstrument, "%v", memorymap.Summary())
		}

	case cmdCPU:
		action, ok := tokens.Get()
		if ok {
			switch strings.ToUpper(action) {
			case "SET":
				target, _ := tokens.Get()
				value, _ := tokens.Get()

				target = strings.ToUpper(target)
				if target == "PC" {
					// program counter can be a 16 bit number
					v, err := strconv.ParseUint(value, 0, 16)
					if err != nil {
						dbg.printLine(terminal.StyleError, "value must be a positive 16 number")
					}

					dbg.VCS.CPU.PC.Load(uint16(v))
				} else {
					// 6507 registers are 8 bit
					v, err := strconv.ParseUint(value, 0, 8)
					if err != nil {
						dbg.printLine(terminal.StyleError, "value must be a positive 8 number")
					}

					var reg *registers.Register
					switch strings.ToUpper(target) {
					case "A":
						reg = dbg.VCS.CPU.A
					case "X":
						reg = dbg.VCS.CPU.X
					case "Y":
						reg = dbg.VCS.CPU.Y
					case "SP":
						reg = dbg.VCS.CPU.SP
					}

					reg.Load(uint8(v))
				}

			default:
				// already caught by command line ValidateTokens()
			}
		} else {
			dbg.printLine(terminal.StyleInstrument, dbg.VCS.CPU.String())
		}

	case cmdPeek:
		// get first address token
		a, ok := tokens.Get()

		for ok {
			// perform peek
			ai, err := dbg.dbgmem.peek(a)
			if err != nil {
				dbg.printLine(terminal.StyleError, "%s", err)
			} else {
				dbg.printLine(terminal.StyleInstrument, ai.String())
			}

			// loop through all addresses
			a, ok = tokens.Get()
		}

	case cmdPoke:
		// get address token
		a, _ := tokens.Get()

		// convert address. note that the calls to dbgmem.poke() also call
		// mapAddress(). the reason we map the address here is because we want
		// a numeric address that we can iterate with in the for loop below.
		// simply converting to a number is no good because we want the user to
		// be able to specify an address by name, so we may as well just call
		// mapAddress, even if it does seem redundant.
		ai := dbg.dbgmem.mapAddress(a, false)
		if ai == nil {
			dbg.printLine(terminal.StyleError, errors.New(errors.UnpokeableAddress, a).Error())
			return false, nil
		}
		addr := ai.mappedAddress

		// get (first) value token
		v, ok := tokens.Get()

		for ok {
			val, err := strconv.ParseUint(v, 0, 8)
			if err != nil {
				dbg.printLine(terminal.StyleError, "value must be an 8 bit number (%s)", v)
				v, ok = tokens.Get()
				continue // for loop (without advancing address)
			}

			ai, err := dbg.dbgmem.poke(addr, uint8(val))
			if err != nil {
				dbg.printLine(terminal.StyleError, "%s", err)
			} else {
				dbg.printLine(terminal.StyleInstrument, ai.String())
			}

			// loop through all values
			v, ok = tokens.Get()
			addr++
		}

	case cmdRAM:
		dbg.printLine(terminal.StyleInstrument, dbg.VCS.Mem.RAM.String())

	case cmdTimer:
		dbg.printLine(terminal.StyleInstrument, dbg.VCS.RIOT.Timer.String())

	case cmdTIA:
		dbg.printLine(terminal.StyleInstrument, dbg.VCS.TIA.String())

	case cmdAudio:
		dbg.printLine(terminal.StyleInstrument, dbg.VCS.TIA.Audio.String())

	case cmdTV:
		option, ok := tokens.Get()
		if ok {
			option = strings.ToUpper(option)
			switch option {
			case "SPEC":
				newspec, ok := tokens.Get()
				if ok {
					// unknown specifciations already handled by ValidateTokens()
					err := dbg.tv.SetSpec(newspec)
					if err != nil {
						return false, err
					}
				}

				spec, auto := dbg.tv.GetSpec()
				s := strings.Builder{}
				s.WriteString(spec.ID)
				if auto {
					s.WriteString(" (auto)")
				}
				dbg.printLine(terminal.StyleInstrument, s.String())
			default:
				// already caught by command line ValidateTokens()
			}
		} else {
			dbg.printLine(terminal.StyleInstrument, dbg.tv.String())
		}

	// information about the machine (sprites, playfield)
	case cmdPlayer:
		plyr := -1

		arg, _ := tokens.Get()
		switch arg {
		case "0":
			plyr = 0
		case "1":
			plyr = 1
		}

		switch plyr {
		case 0:
			dbg.printLine(terminal.StyleInstrument, dbg.VCS.TIA.Video.Player0.String())

		case 1:
			dbg.printLine(terminal.StyleInstrument, dbg.VCS.TIA.Video.Player1.String())

		default:
			dbg.printLine(terminal.StyleInstrument, dbg.VCS.TIA.Video.Player0.String())
			dbg.printLine(terminal.StyleInstrument, dbg.VCS.TIA.Video.Player1.String())
		}

	case cmdMissile:
		miss := -1

		arg, _ := tokens.Get()
		switch arg {
		case "0":
			miss = 0
		case "1":
			miss = 1
		}

		switch miss {
		case 0:
			dbg.printLine(terminal.StyleInstrument, dbg.VCS.TIA.Video.Missile0.String())

		case 1:
			dbg.printLine(terminal.StyleInstrument, dbg.VCS.TIA.Video.Missile1.String())

		default:
			dbg.printLine(terminal.StyleInstrument, dbg.VCS.TIA.Video.Missile0.String())
			dbg.printLine(terminal.StyleInstrument, dbg.VCS.TIA.Video.Missile1.String())
		}

	case cmdBall:
		dbg.printLine(terminal.StyleInstrument, dbg.VCS.TIA.Video.Ball.String())

	case cmdPlayfield:
		dbg.printLine(terminal.StyleInstrument, dbg.VCS.TIA.Video.Playfield.String())

	case cmdDisplay:
		var err error

		action, _ := tokens.Get()
		action = strings.ToUpper(action)
		switch action {
		case "ON":
			err = dbg.scr.ReqFeature(gui.ReqSetVisibility, true)

		case "OFF":
			err = dbg.scr.ReqFeature(gui.ReqSetVisibility, false)

		case "SCALE":
			scl, ok := tokens.Get()
			if !ok {
				return false, errors.New(errors.CommandError, fmt.Sprintf("value required for %s %s", cmdDisplay, action))
			}

			scale, err := strconv.ParseFloat(scl, 32)
			if err != nil {
				return false, errors.New(errors.CommandError, fmt.Sprintf("%s %s value not valid (%s)", cmdDisplay, action, scl))
			}

			err = dbg.scr.ReqFeature(gui.ReqSetScale, float32(scale))

		case "MASKING":
			action, _ := tokens.Get()
			action = strings.ToUpper(action)
			switch action {
			case "OFF":
				err = dbg.scr.ReqFeature(gui.ReqSetCropping, false)
			case "ON":
				err = dbg.scr.ReqFeature(gui.ReqSetCropping, true)
			default:
				err = dbg.scr.ReqFeature(gui.ReqToggleCropping)
			}

		case "DBG":
			action, _ := tokens.Get()
			action = strings.ToUpper(action)
			switch action {
			case "OFF":
				err = dbg.scr.ReqFeature(gui.ReqSetDbgColors, false)
			case "ON":
				err = dbg.scr.ReqFeature(gui.ReqSetDbgColors, true)
			default:
				err = dbg.scr.ReqFeature(gui.ReqToggleDbgColors)
			}
		case "OVERLAY":
			action, _ := tokens.Get()
			action = strings.ToUpper(action)
			switch action {
			case "OFF":
				err = dbg.scr.ReqFeature(gui.ReqSetOverlay, false)
			case "ON":
				err = dbg.scr.ReqFeature(gui.ReqSetOverlay, true)
			default:
				err = dbg.scr.ReqFeature(gui.ReqToggleOverlay)
			}
		default:
			err = dbg.scr.ReqFeature(gui.ReqToggleVisibility)
			if err != nil {
				return false, err
			}
		}

		if err != nil {
			if errors.Is(err, errors.UnsupportedGUIRequest) {
				return false, errors.New(errors.CommandError, fmt.Sprintf("display does not support feature %s", action))
			}
			return false, err
		}

	case cmdController:
		player, _ := tokens.Get()

		var p *input.HandController
		switch player {
		case "0":
			p = dbg.VCS.HandController0
		case "1":
			p = dbg.VCS.HandController1
		}

		controller, ok := tokens.Get()
		if ok {
			switch strings.ToLower(controller) {
			case "auto":
				p.SetAuto(true)
			case "noauto":
				p.SetAuto(false)
			case "joystick":
				p.SwitchType(input.JoystickType)
			case "paddle":
				p.SwitchType(input.PaddleType)
			case "keypad":
				p.SwitchType(input.KeypadType)
			}
		}

		s := strings.Builder{}

		switch p.ControllerType {
		case input.JoystickType:
			s.WriteString("Joystick")
		case input.PaddleType:
			s.WriteString("Paddle")
		case input.KeypadType:
			s.WriteString("Keypad")
		default:
			s.WriteString("Unknown")
		}

		if p.AutoControllerType {
			s.WriteString(" (auto)")
		}

		dbg.printLine(terminal.StyleFeedback, s.String())

	case cmdPanel:
		mode, ok := tokens.Get()
		if !ok {
			dbg.printLine(terminal.StyleInstrument, dbg.VCS.Panel.String())
			return false, nil
		}

		switch strings.ToUpper(mode) {
		case "TOGGLE":
			arg, _ := tokens.Get()
			switch strings.ToUpper(arg) {
			case "P0":
				dbg.VCS.Panel.Handle(input.PanelTogglePlayer0Pro, nil)
			case "P1":
				dbg.VCS.Panel.Handle(input.PanelTogglePlayer1Pro, nil)
			case "COL":
				dbg.VCS.Panel.Handle(input.PanelToggleColor, nil)
			}
		case "SET":
			arg, _ := tokens.Get()
			switch strings.ToUpper(arg) {
			case "P0PRO":
				dbg.VCS.Panel.Handle(input.PanelSetPlayer0Pro, true)
			case "P1PRO":
				dbg.VCS.Panel.Handle(input.PanelSetPlayer1Pro, true)
			case "P0AM":
				dbg.VCS.Panel.Handle(input.PanelSetPlayer0Pro, false)
			case "P1AM":
				dbg.VCS.Panel.Handle(input.PanelSetPlayer1Pro, false)
			case "COL":
				dbg.VCS.Panel.Handle(input.PanelSetColor, true)
			case "BW":
				dbg.VCS.Panel.Handle(input.PanelSetColor, false)
			}
		case "HOLD":
			arg, _ := tokens.Get()
			switch strings.ToUpper(arg) {
			case "SELECT":
				dbg.VCS.Panel.Handle(input.PanelSelect, true)
			case "RESET":
				dbg.VCS.Panel.Handle(input.PanelReset, true)
			}
		case "RELEASE":
			arg, _ := tokens.Get()
			switch strings.ToUpper(arg) {
			case "SELECT":
				dbg.VCS.Panel.Handle(input.PanelSelect, false)
			case "RESET":
				dbg.VCS.Panel.Handle(input.PanelReset, false)
			}
		}
		dbg.printLine(terminal.StyleInstrument, dbg.VCS.Panel.String())

	case cmdJoystick:
		var err error

		stick, _ := tokens.Get()
		action, _ := tokens.Get()

		var event input.Event
		var value input.EventData

		switch strings.ToUpper(action) {
		case "FIRE":
			event = input.Fire
			value = true
		case "UP":
			event = input.Up
			value = true
		case "DOWN":
			event = input.Down
			value = true
		case "LEFT":
			event = input.Left
			value = true
		case "RIGHT":
			event = input.Right
			value = true

		case "NOFIRE":
			event = input.Fire
			value = false
		case "NOUP":
			event = input.Up
			value = false
		case "NODOWN":
			event = input.Down
			value = false
		case "NOLEFT":
			event = input.Left
			value = false
		case "NORIGHT":
			event = input.Right
			value = false
		}

		n, _ := strconv.Atoi(stick)
		switch n {
		case 0:
			err = dbg.VCS.HandController0.Handle(event, value)
		case 1:
			err = dbg.VCS.HandController1.Handle(event, value)
		}

		if err != nil {
			return false, err
		}

	case cmdKeypad:
		var err error

		pad, _ := tokens.Get()
		key, _ := tokens.Get()

		n, _ := strconv.Atoi(pad)
		switch n {
		case 0:
			if strings.ToUpper(key) == "NONE" {
				err = dbg.VCS.HandController0.Handle(input.KeypadUp, nil)
			} else {
				err = dbg.VCS.HandController0.Handle(input.KeypadDown, rune(key[0]))
			}
		case 1:
			if strings.ToUpper(key) == "NONE" {
				err = dbg.VCS.HandController1.Handle(input.KeypadUp, nil)
			} else {
				err = dbg.VCS.HandController1.Handle(input.KeypadDown, rune(key[0]))
			}
		}

		if err != nil {
			return false, err
		}

	case cmdBreak:
		err := dbg.breakpoints.parseCommand(tokens)
		if err != nil {
			return false, errors.New(errors.CommandError, err)
		}

	case cmdTrap:
		err := dbg.traps.parseCommand(tokens)
		if err != nil {
			return false, errors.New(errors.CommandError, err)
		}

	case cmdWatch:
		err := dbg.watches.parseCommand(tokens)
		if err != nil {
			return false, errors.New(errors.CommandError, err)
		}

	case cmdTrace:
		err := dbg.traces.parseCommand(tokens)
		if err != nil {
			return false, errors.New(errors.CommandError, err)
		}

	case cmdList:
		list, _ := tokens.Get()
		list = strings.ToUpper(list)
		switch list {
		case "BREAKS":
			dbg.breakpoints.list()
		case "TRAPS":
			dbg.traps.list()
		case "WATCHES":
			dbg.watches.list()
		case "TRACES":
			dbg.traces.list()
		case "ALL":
			dbg.breakpoints.list()
			dbg.traps.list()
			dbg.watches.list()
			dbg.traces.list()
		default:
			// already caught by command line ValidateTokens()
		}

	case cmdDrop:
		drop, _ := tokens.Get()

		s, _ := tokens.Get()
		num, err := strconv.Atoi(s)
		if err != nil {
			return false, errors.New(errors.CommandError, fmt.Sprintf("drop attribute must be a number (%s)", s))
		}

		drop = strings.ToUpper(drop)
		switch drop {
		case "BREAK":
			err := dbg.breakpoints.drop(num)
			if err != nil {
				return false, err
			}
			dbg.printLine(terminal.StyleFeedback, "breakpoint #%d dropped", num)
		case "TRAP":
			err := dbg.traps.drop(num)
			if err != nil {
				return false, err
			}
			dbg.printLine(terminal.StyleFeedback, "trap #%d dropped", num)
		case "WATCH":
			err := dbg.watches.drop(num)
			if err != nil {
				return false, err
			}
			dbg.printLine(terminal.StyleFeedback, "watch #%d dropped", num)
		case "TRACE":
			err := dbg.traces.drop(num)
			if err != nil {
				return false, err
			}
			dbg.printLine(terminal.StyleFeedback, "trace #%d dropped", num)
		default:
			// already caught by command line ValidateTokens()
		}

	case cmdClear:
		clear, _ := tokens.Get()
		clear = strings.ToUpper(clear)
		switch clear {
		case "BREAKS":
			dbg.breakpoints.clear()
			dbg.printLine(terminal.StyleFeedback, "breakpoints cleared")
		case "TRAPS":
			dbg.traps.clear()
			dbg.printLine(terminal.StyleFeedback, "traps cleared")
		case "WATCHES":
			dbg.watches.clear()
			dbg.printLine(terminal.StyleFeedback, "watches cleared")
		case "TRACES":
			dbg.traces.clear()
			dbg.printLine(terminal.StyleFeedback, "traces cleared")
		case "ALL":
			dbg.breakpoints.clear()
			dbg.traps.clear()
			dbg.watches.clear()
			dbg.traces.clear()
			dbg.printLine(terminal.StyleFeedback, "breakpoints, traps, watches and traces cleared")
		default:
			// already caught by command line ValidateTokens()
		}

	case cmdPref:
		action, ok := tokens.Get()

		if !ok {
			dbg.printLine(terminal.StyleFeedback, dbg.Prefs.String())
			dbg.printLine(terminal.StyleFeedback, dbg.Disasm.Prefs.String())
			return false, nil
		}

		switch action {
		case "LOAD":
			err := dbg.Prefs.load()
			if err != nil {
				return false, errors.New(errors.CommandError, err)
			}
			err = dbg.Disasm.Prefs.Load()
			if err != nil {
				return false, errors.New(errors.CommandError, err)
			}

		case "SAVE":
			err := dbg.Prefs.save()
			if err != nil {
				return false, errors.New(errors.CommandError, err)
			}
			err = dbg.Disasm.Prefs.Save()
			if err != nil {
				return false, errors.New(errors.CommandError, err)
			}
		}

		option, _ := tokens.Get()

		option = strings.ToUpper(option)
		switch option {
		case "RANDSTART":
			switch action {
			case "SET":
				dbg.Prefs.RandomState.Set(true)
			case "UNSET":
				dbg.Prefs.RandomState.Set(false)
			case "TOGGLE":
				v := dbg.Prefs.RandomState.Get().(bool)
				dbg.Prefs.RandomState.Set(!v)
			}
		case "RANDPINS":
			switch action {
			case "SET":
				dbg.Prefs.RandomPins.Set(true)
			case "UNSET":
				dbg.Prefs.RandomPins.Set(false)
			case "TOGGLE":
				v := dbg.Prefs.RandomPins.Get().(bool)
				dbg.Prefs.RandomPins.Set(!v)
			}
		case "FXXXMIRROR":
			switch action {
			case "SET":
				dbg.Disasm.Prefs.FxxxMirror.Set(true)
			case "UNSET":
				dbg.Disasm.Prefs.FxxxMirror.Set(false)
			case "TOGGLE":
				v := dbg.Disasm.Prefs.FxxxMirror.Get().(bool)
				dbg.Disasm.Prefs.FxxxMirror.Set(!v)
			}
		}

	case cmdLog:
		option, ok := tokens.Get()
		if ok {
			switch option {
			case "CLEAR":
				logger.Clear()
			}
		} else {
			s := &strings.Builder{}
			if logger.Write(s) {
				dbg.printLine(terminal.StyleLog, s.String())
			} else {
				dbg.printLine(terminal.StyleFeedback, "log is empty")
			}
		}
	}

	return false, nil
}
