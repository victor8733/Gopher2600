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

package vcs

import (
	"github.com/jetsetilly/gopher2600/hardware/memory/memorymap"
)

// NewRIOT is the preferred method of initialisation for the RIOT memory area
func NewRIOT() *ChipMemory {
	area := &ChipMemory{
		origin: memorymap.OriginRIOT,
		memtop: memorymap.MemtopRIOT,
	}

	// allocation the minimal amount of memory
	area.memory = make([]uint8, area.memtop-area.origin+1)

	// SWCHA set on startup by NewHandController0() and NewHandController1()

	// SWCHB set on startup NewPanel()

	return area
}
