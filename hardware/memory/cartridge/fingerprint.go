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
	"github.com/jetsetilly/gopher2600/hardware/memory/cartridge/harmony"
	"github.com/jetsetilly/gopher2600/hardware/memory/cartridge/supercharger"
)

func fingerprint3ePlus(b []byte) bool {
	// 3e is similar to tigervision, a key difference being that it uses 0x3e
	// to switch ram, in addition to 0x3f for switching banks.
	//
	// postulating that the fingerprint method can be the same except for the
	// write address.

	threshold3e := 5
	threshold3f := 5
	for i := range b {
		if b[i] == 0x85 && b[i+1] == 0x3e {
			threshold3e--
		}
		if b[i] == 0x85 && b[i+1] == 0x3f {
			threshold3f--
		}
		if threshold3e <= 0 && threshold3f <= 0 {
			return true
		}
	}
	return false
}

func fingerprintMnetwork(b []byte) bool {
	threshold := 2
	for i := 0; i < len(b)-3; i++ {
		if b[i] == 0x7e && b[i+1] == 0x66 && b[i+2] == 0x66 && b[i+3] == 0x66 {
			threshold--
		}
		if threshold == 0 {
			return true
		}
	}

	return false
}

func fingerprintParkerBros(b []byte) bool {
	// fingerprint patterns taken from Stella CartDetector.cxx
	for i := 0; i <= len(b)-3; i++ {
		if (b[i] == 0x8d && b[i+1] == 0xe0 && b[i+2] == 0x1f) ||
			(b[i] == 0x8d && b[i+1] == 0xe0 && b[i+2] == 0x5f) ||
			(b[i] == 0x8d && b[i+1] == 0xe9 && b[i+2] == 0xff) ||
			(b[i] == 0x0c && b[i+1] == 0xe0 && b[i+2] == 0x1f) ||
			(b[i] == 0xad && b[i+1] == 0xe0 && b[i+2] == 0x1f) ||
			(b[i] == 0xad && b[i+1] == 0xe9 && b[i+2] == 0xff) ||
			(b[i] == 0xad && b[i+1] == 0xed && b[i+2] == 0xff) ||
			(b[i] == 0xad && b[i+1] == 0xf3 && b[i+2] == 0xbf) {
			return true
		}

	}

	return false
}

func fingerprintHarmony(b []byte) bool {
	return b[0x20] == 0x1e && b[0x21] == 0xab && b[0x22] == 0xad && b[0x23] == 0x10
}

func fingerprintSuperchargerFastLoad(b []byte) bool {
	l := len(b)
	return l == 8448 || l == 25344 || l == 33792
}

func fingerprintTigervision(b []byte) bool {
	// tigervision cartridges change banks by writing to memory address 0x3f. we
	// can hypothesize that these types of cartridges will have that instruction
	// sequence "85 3f" many times in a ROM whereas other cartridge types will not

	threshold := 5
	for i := 0; i < len(b)-1; i++ {
		if b[i] == 0x85 && b[i+1] == 0x3f {
			threshold--
		}
		if threshold == 0 {
			return true
		}
	}
	return false
}

func fingerprint8k(data []byte) func([]byte) (cartMapper, error) {
	if fingerprintTigervision(data) {
		return newTigervision
	}

	if fingerprintParkerBros(data) {
		return newParkerBros
	}

	return newAtari8k
}

func fingerprint16k(data []byte) func([]byte) (cartMapper, error) {
	if fingerprintTigervision(data) {
		return newTigervision
	}

	if fingerprintMnetwork(data) {
		return newMnetwork
	}

	return newAtari16k
}

func fingerprint32k(data []byte) func([]byte) (cartMapper, error) {
	if fingerprintTigervision(data) {
		return newTigervision
	}

	return newAtari32k
}

func (cart *Cartridge) fingerprint(data []byte) error {
	var err error

	if fingerprintHarmony(data) {
		// !!TODO: this might be a CFDJ cartridge. check for that.
		cart.mapper, err = harmony.NewDPCplus(data)
		return err
	}

	if fingerprintSuperchargerFastLoad(data) {
		cart.mapper, err = supercharger.NewSupercharger(data)
		return err
	}

	if fingerprint3ePlus(data) {
		cart.mapper, err = new3ePlus(data)
		return err
	}

	switch len(data) {
	case 2048:
		cart.mapper, err = newAtari2k(data)
		if err != nil {
			return err
		}

	case 4096:
		cart.mapper, err = newAtari4k(data)
		if err != nil {
			return err
		}

	case 8192:
		cart.mapper, err = fingerprint8k(data)(data)
		if err != nil {
			return err
		}

	case 10240:
		fallthrough

	case 10495:
		cart.mapper, err = newDPC(data)
		if err != nil {
			return err
		}

	case 12288:
		cart.mapper, err = newCBS(data)
		if err != nil {
			return err
		}

	case 16384:
		cart.mapper, err = fingerprint16k(data)(data)
		if err != nil {
			return err
		}

	case 32768:
		cart.mapper, err = fingerprint32k(data)(data)
		if err != nil {
			return err
		}

	case 65536:
		return errors.New(errors.CartridgeError, "65536 bytes not yet supported")

	default:
		return errors.New(errors.CartridgeError, fmt.Sprintf("unrecognised cartridge size (%d bytes)", len(data)))
	}

	// if cartridge mapper implements the optionalSuperChip interface then try
	// to add the additional RAM
	if superchip, ok := cart.mapper.(optionalSuperchip); ok {
		superchip.addSuperchip()
	}

	return nil
}
