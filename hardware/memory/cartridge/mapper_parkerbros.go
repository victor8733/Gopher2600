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

package cartridge

import (
	"fmt"

	"github.com/jetsetilly/gopher2600/errors"
	"github.com/jetsetilly/gopher2600/hardware/memory/cartridge/banks"
	"github.com/jetsetilly/gopher2600/hardware/memory/memorymap"
)

// from bankswitch_sizes.txt:
//
// -E0: Parker Brothers was the main user of this method.  This cart is
// segmented into 4 1K segments.  Each segment can point to one 1K slice of the
// ROM image.  You select the desired 1K slice by accessing 1FE0 to 1FE7 for
// the first 1K (1FE0 selects slice 0, 1FE1 selects slice 1, etc).  1FE8 to
// 1FEF selects the slice for the second 1K, and 1FF0 to 1FF8 selects the slice
// for the third 1K.  The last 1K always points to the last 1K of the ROM image
// so that the cart always starts up in the exact same place.

// parkerBros implements the cartMapper interface.
//  o Montezuma's Revenge
//  o Lord of the Rings
//  o etc.
type parkerBros struct {
	mappingID   string
	description string

	// parkerBros cartridges have 8 banks of 1024 bytes
	bankSize int
	banks    [][]uint8

	// parkerBros cartridges divide memory into 4 segments
	//  o the last segment always points to the last bank
	//  o the other segments can point to any one of the eight banks in the ROM
	//		(including the last bank)
	segment [4]int
}

func newParkerBros(data []byte) (cartMapper, error) {
	cart := &parkerBros{
		mappingID:   "E0",
		description: "parker bros",
		bankSize:    1024,
	}

	cart.banks = make([][]uint8, cart.NumBanks())

	if len(data) != cart.bankSize*cart.NumBanks() {
		return nil, errors.New(errors.CartridgeError, fmt.Sprintf("%s: wrong number of bytes in the cartridge data", cart.mappingID))
	}

	for k := 0; k < cart.NumBanks(); k++ {
		cart.banks[k] = make([]uint8, cart.bankSize)
		offset := k * cart.bankSize
		copy(cart.banks[k], data[offset:offset+cart.bankSize])
	}

	cart.Initialise()

	return cart, nil
}

func (cart parkerBros) String() string {
	return fmt.Sprintf("%s [%s] Banks: %d, %d, %d, %d", cart.mappingID, cart.description, cart.segment[0], cart.segment[1], cart.segment[2], cart.segment[3])
}

// ID implements the cartMapper interface
func (cart parkerBros) ID() string {
	return cart.mappingID
}

// Initialise implements the cartMapper interface
func (cart *parkerBros) Initialise() {
	cart.segment[0] = cart.NumBanks() - 4
	cart.segment[1] = cart.NumBanks() - 3
	cart.segment[2] = cart.NumBanks() - 2
	cart.segment[3] = cart.NumBanks() - 1
}

// Read implements the cartMapper interface
func (cart *parkerBros) Read(addr uint16, passive bool) (uint8, error) {
	if cart.hotspot(addr, passive) {
		return 0, nil
	}

	var data uint8
	if addr >= 0x0000 && addr <= 0x03ff {
		data = cart.banks[cart.segment[0]][addr&0x03ff]
	} else if addr >= 0x0400 && addr <= 0x07ff {
		data = cart.banks[cart.segment[1]][addr&0x03ff]
	} else if addr >= 0x0800 && addr <= 0x0bff {
		data = cart.banks[cart.segment[2]][addr&0x03ff]
	} else if addr >= 0x0c00 && addr <= 0x0fff {
		data = cart.banks[cart.segment[3]][addr&0x03ff]
	}
	return data, nil
}

// Write implements the cartMapper interface
func (cart *parkerBros) Write(addr uint16, data uint8, passive bool, poke bool) error {
	if cart.hotspot(addr, passive) {
		return nil
	}

	if poke {
		if addr >= 0x0000 && addr <= 0x03ff {
			cart.banks[cart.segment[0]][addr&0x3dd] = data
		} else if addr >= 0x0400 && addr <= 0x07ff {
			cart.banks[cart.segment[1]][addr&0x3dd] = data
		} else if addr >= 0x0800 && addr <= 0x0bff {
			cart.banks[cart.segment[2]][addr&0x3dd] = data
		} else if addr >= 0x0c00 && addr <= 0x0fff {
			cart.banks[cart.segment[3]][addr&0x3dd] = data
		}
	}

	return errors.New(errors.MemoryBusError, addr)
}

// bankswitch on hotspot access
func (cart *parkerBros) hotspot(addr uint16, passive bool) bool {
	if addr >= 0xfe0 && addr <= 0xff7 {
		if passive {
			return true
		}

		switch addr {
		// segment 0
		case 0x0fe0:
			cart.segment[0] = 0
		case 0x0fe1:
			cart.segment[0] = 1
		case 0x0fe2:
			cart.segment[0] = 2
		case 0x0fe3:
			cart.segment[0] = 3
		case 0x0fe4:
			cart.segment[0] = 4
		case 0x0fe5:
			cart.segment[0] = 5
		case 0x0fe6:
			cart.segment[0] = 6
		case 0x0fe7:
			cart.segment[0] = 7

		// segment 1
		case 0x0fe8:
			cart.segment[1] = 0
		case 0x0fe9:
			cart.segment[1] = 1
		case 0x0fea:
			cart.segment[1] = 2
		case 0x0feb:
			cart.segment[1] = 3
		case 0x0fec:
			cart.segment[1] = 4
		case 0x0fed:
			cart.segment[1] = 5
		case 0x0fee:
			cart.segment[1] = 6
		case 0x0fef:
			cart.segment[1] = 7

		// segment 2
		case 0x0ff0:
			cart.segment[2] = 0
		case 0x0ff1:
			cart.segment[2] = 1
		case 0x0ff2:
			cart.segment[2] = 2
		case 0x0ff3:
			cart.segment[2] = 3
		case 0x0ff4:
			cart.segment[2] = 4
		case 0x0ff5:
			cart.segment[2] = 5
		case 0x0ff6:
			cart.segment[2] = 6
		case 0x0ff7:
			cart.segment[2] = 7
		}

		return true
	}

	return false
}

// NumBanks implements the cartMapper interface
func (cart parkerBros) NumBanks() int {
	return 8
}

// GetBank implements the cartMapper interface
func (cart parkerBros) GetBank(addr uint16) banks.Details {
	var seg int
	if addr >= 0x0000 && addr <= 0x03ff {
		seg = 0
	} else if addr >= 0x0400 && addr <= 0x07ff {
		seg = 1
	} else if addr >= 0x0800 && addr <= 0x0bff {
		seg = 2
	} else { // remaining address is between 0x0c00 and 0x0fff
		seg = 3
	}

	return banks.Details{Number: cart.segment[seg], IsRAM: false, Segment: seg}
}

// Patch implements the cartMapper interface
func (cart *parkerBros) Patch(offset int, data uint8) error {
	if offset >= cart.bankSize*len(cart.banks) {
		return errors.New(errors.CartridgePatchOOB, offset)
	}

	bank := int(offset) / cart.bankSize
	offset = offset % cart.bankSize
	cart.banks[bank][offset] = data
	return nil
}

// Listen implements the cartMapper interface
func (cart *parkerBros) Listen(_ uint16, _ uint8) {
}

// Step implements the cartMapper interface
func (cart *parkerBros) Step() {
}

// IterateBank implemnts the disassemble interface
func (cart parkerBros) IterateBanks(prev *banks.Content) *banks.Content {
	b := prev.Number + 1
	if b >= 0 && b <= 6 {
		// banks 0 to 6 can occupy any of the three segments
		return &banks.Content{Number: b,
			Data: cart.banks[b],
			Origins: []uint16{
				memorymap.OriginCart,
				memorymap.OriginCart + uint16(cart.bankSize),
				memorymap.OriginCart + uint16(cart.bankSize)*2,
			},
		}
	} else if b == 7 {
		// bank 7 can occupy any of the four segments (the last segment always
		// points to bank 7 but bank 7 can also be in another segment at the
		// same time)
		return &banks.Content{Number: b,
			Data: cart.banks[b],
			Origins: []uint16{
				memorymap.OriginCart,
				memorymap.OriginCart + uint16(cart.bankSize),
				memorymap.OriginCart + uint16(cart.bankSize)*2,
				memorymap.OriginCart + uint16(cart.bankSize)*3,
			},
		}
	}
	return nil
}
