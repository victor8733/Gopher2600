package debugger

import (
	"fmt"
	"gopher2600/errors"
	"gopher2600/hardware/memory"
	"gopher2600/hardware/memory/vcssymbols"
	"gopher2600/symbols"
	"strings"
)

// memoryDebug is a front-end to the real VCS memory. this additonal memory
// layer allows addressing by symbols.
type memoryDebug struct {
	vcsmem *memory.VCSMemory

	// symbols.Table instance can change after we've initialised with
	// newMemoryDebug(), so we need a pointer to a pointer
	symtable **symbols.Table
}

func newMemoryDebug(dbg *Debugger) *memoryDebug {
	mem := new(memoryDebug)
	mem.vcsmem = dbg.vcs.Mem
	mem.symtable = &dbg.disasm.Symtable
	return mem
}

// mapAddress allows addressing by symbols in addition to numerically
func (mem memoryDebug) mapAddress(address interface{}, cpuPerspective bool) (uint16, error) {
	var mapped bool
	var ma uint16
	var symbolTable map[uint16]string

	if cpuPerspective {
		symbolTable = (*mem.symtable).ReadSymbols
	} else {
		symbolTable = vcssymbols.WriteSymbols
	}

	switch address := address.(type) {
	case uint16:
		ma = mem.vcsmem.MapAddress(uint16(address), true)
		mapped = true
	case string:
		// search for symbolic address in standard vcs read symbols
		for a, sym := range symbolTable {
			if sym == address {
				ma = a
				mapped = true
				break // for loop
			}
		}

		// try again with an uppercase label
		address = strings.ToUpper(address)
		for a, sym := range symbolTable {
			if sym == address {
				ma = a
				mapped = true
				break // for loop
			}
		}
	}

	if !mapped {
		return 0, errors.NewGopherError(errors.UnrecognisedAddress, address)
	}

	return ma, nil
}

// Peek returns the contents of the memory address, without triggering any side
// effects. returns:
//  o value
//  o mapped address
//  o area name
//  o address label
//  o error
func (mem memoryDebug) peek(address interface{}) (uint8, uint16, string, string, error) {
	ma, err := mem.mapAddress(address, true)
	if err != nil {
		return 0, 0, "", "", err
	}

	area, present := mem.vcsmem.Memmap[ma]
	if !present {
		panic(fmt.Errorf("%04x not mapped correctly", address))
	}

	return area.Peek(ma)
}

// Poke writes a value at the address
func (mem memoryDebug) poke(address interface{}, value uint8) error {
	ma, err := mem.mapAddress(address, true)
	if err != nil {
		return err
	}

	area, present := mem.vcsmem.Memmap[ma]
	if !present {
		panic(fmt.Errorf("%04x not mapped correctly", address))
	}

	return area.Poke(ma, value)
}