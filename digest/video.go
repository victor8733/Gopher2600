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

package digest

import (
	"crypto/sha1"
	"fmt"

	"github.com/jetsetilly/gopher2600/errors"
	"github.com/jetsetilly/gopher2600/television"
)

// Video is an implementation of the television.PixelRenderer interface with an
// embedded television for convenience. It generates a SHA-1 value of the image
// every frame. it does not display the image anywhere.
//
// Note that the use of SHA-1 is fine for this application because this is not
// a cryptographic task.
type Video struct {
	television.Television
	spec     *television.Specification
	digest   [sha1.Size]byte
	pixels   []byte
	frameNum int
}

const pixelDepth = 3

// NewVideo initialises a new instance of DigestTV. For convenience, the
// television argument can be nil, in which case an instance of
// StellaTelevision will be created.
func NewVideo(tv television.Television) (*Video, error) {
	// set up digest tv
	dig := &Video{Television: tv}

	// register ourselves as a television.Renderer
	dig.AddPixelRenderer(dig)

	// length of pixels array contains enough room for the previous frames
	// digest value
	l := len(dig.digest)

	// allocate enough pixels for entire frame
	dig.spec, _ = dig.GetSpec()
	l += ((television.HorizClksScanline + 1) * (dig.spec.ScanlinesTotal + 1) * pixelDepth)
	dig.pixels = make([]byte, l)

	return dig, nil
}

// Hash implements digest.Digest interface
func (dig Video) Hash() string {
	return fmt.Sprintf("%x", dig.digest)
}

// ResetDigest implements digest.Digest interface
func (dig *Video) ResetDigest() {
	for i := range dig.digest {
		dig.digest[i] = 0
	}
}

// Resize implements television.PixelRenderer interface
//
// In this implementation we only handle specification changes. This means the
// digest is immune from changes to the frame resizing method used by the
// television implementation. Changes to how the specification is flipped might
// cause comparison failures however.
func (dig *Video) Resize(spec *television.Specification, _, _ int) error {
	if spec == dig.spec {
		return nil
	}

	// allocate enough pixels for entire frame
	dig.spec = spec
	l := len(dig.digest)
	l += ((television.HorizClksScanline + 1) * (spec.ScanlinesTotal + 1) * pixelDepth)
	dig.pixels = make([]byte, l)

	return nil
}

// NewFrame implements television.PixelRenderer interface
func (dig *Video) NewFrame(frameNum int, _ bool) error {
	// chain fingerprints by copying the value of the last fingerprint
	// to the head of the video data
	n := copy(dig.pixels, dig.digest[:])
	if n != len(dig.digest) {
		return errors.New(errors.VideoDigest, fmt.Sprintf("digest error during new frame"))
	}
	dig.digest = sha1.Sum(dig.pixels)
	dig.frameNum = frameNum
	return nil
}

// NewScanline implements television.PixelRenderer interface
func (dig *Video) NewScanline(scanline int) error {
	return nil
}

// SetPixel implements television.PixelRenderer interface
func (dig *Video) SetPixel(x, y int, red, green, blue byte, vblank bool) error {
	// preserve the first few bytes for a chained fingerprint
	i := len(dig.digest)
	i += television.HorizClksScanline * y * pixelDepth
	i += x * pixelDepth

	if i <= len(dig.pixels)-pixelDepth {
		// setting every pixel regardless of vblank value
		dig.pixels[i] = red
		dig.pixels[i+1] = green
		dig.pixels[i+2] = blue
	}

	return nil
}

// EndRendering implements television.PixelRenderer interface
func (dig *Video) EndRendering() error {
	return nil
}
