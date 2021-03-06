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

package television

// FrameResizeID identifies the resizing method
type FrameResizeID string

// List of valid values for FrameResizeID
const (
	FrameResizerNone   FrameResizeID = "FrameResizerNone"
	FrameResizerSimple FrameResizeID = "FrameResizerSimple"
)

// !!TODO: more sophisticated resizer implementations

// the resizer interfaces specifies the operations required by a mechanism that
// will alter the visible frame of the television
type resizer interface {
	// the id of the resizer implementation
	id() FrameResizeID

	// examine signal for resizing possibility. called on every Signal()
	examine(tv *television, sig SignalAttributes)

	// commit resizing possiblity. called on every newFrame()
	commit(tv *television) error

	// preapare for next frame
	prepare(tv *television)
}

// nullResizer is the null implemnentation of the resizer interface
type nullResizer struct{}

func (_ nullResizer) id() FrameResizeID { return FrameResizerNone }

func (_ *nullResizer) examine(_ *television, _ SignalAttributes) {}

func (_ *nullResizer) commit(_ *television) error { return nil }

func (_ *nullResizer) prepare(_ *television) {}

// simpleResizer is the simplest functional and non-trivial implementation of
// the resizer interface
type simpleResizer struct {
	top    int
	bottom int
}

func (sr simpleResizer) id() FrameResizeID {
	return FrameResizerSimple
}

func (sr *simpleResizer) examine(tv *television, sig SignalAttributes) {
	// if vblank is off at any point of then extend the bottom of the screen.
	// we'll commit the resize procedure in the newFrame() function
	//
	// comparing against current bottom scanline, rather than ideal bottom
	// scanline of the specification. this means that a screen will never
	// "shrink" until the specification is changed either manually or
	// automatically.
	//
	// we mitigate this by not initiating a resize event until after the setup
	// phase (as quantified by the leadingFrames value). any problems with ROMs
	// that erroneously trigger a resize through rogue frames will have to be
	// dealt with by some sort of count (ie. the new size has to be "held" for
	// N number of frames before we resize). Earlier versions of this file did
	// do that but we removed it due to no evidence that it was required.
	if !sig.VBlank {
		if tv.scanline > sr.bottom {
			sr.bottom = tv.scanline
		}
	}
}

func (sr *simpleResizer) commit(tv *television) error {
	// always perform resize operation
	if tv.syncedFrameNum <= leadingFrames || sr.bottom == tv.bottom {
		return nil
	}

	diff := sr.bottom - tv.bottom

	// reduce top by same amount as bottom
	tv.top -= diff
	if tv.top < 0 {
		tv.top = 0
	}

	// new bottom value is what we detected
	tv.bottom = sr.bottom

	// call Resize() for all attached pixel rendered
	if tv.top < tv.bottom {
		for f := range tv.renderers {
			err := tv.renderers[f].Resize(tv.spec, tv.top, tv.bottom-tv.top)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (sr *simpleResizer) prepare(tv *television) {
	sr.bottom = tv.bottom
}
