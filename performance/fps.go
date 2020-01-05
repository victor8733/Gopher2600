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
//
// *** NOTE: all historical versions of this file, as found in any
// git repository, are also covered by the licence, even when this
// notice is not present ***

package performance

import "gopher2600/television"

// CalcFPS takes the the number of frames and duration and returns the
// frames-per-second and the accuracy of that value as a percentage.
func CalcFPS(tv television.Television, numFrames int, duration float64) (fps float64, accuracy float64) {
	fps = float64(numFrames) / duration
	accuracy = 100 * float64(numFrames) / (duration * float64(tv.GetSpec().FramesPerSecond))
	return fps, accuracy
}
