package video

import (
	"fmt"
	"gopher2600/hardware/tia/delay"
	"gopher2600/hardware/tia/delay/future"
	"gopher2600/hardware/tia/polycounter"
	"math/bits"
	"strings"
)

type playerSprite struct {
	*sprite

	// additional sprite information
	color         uint8
	size          uint8
	reflected     bool
	verticalDelay bool
	gfxDataA      uint8 // GRP0A	(or GRP1A)
	gfxDataB      uint8 // GRP0B	(or GRP1B)

	// we need access to the other player sprite. when we write new gfxData, it
	// triggers the other player's gfxDataPrev value to equal its gfxData --
	// this wasn't clear to me originally and was crystal clear after reading
	// Erik Mooney's post, "48-pixel highres routine explained!"
	otherPlayer *playerSprite

	// the list of color clock states when missile drawing is triggered
	// (in addition to when the sprite's position counter loops back to zero)
	triggerList []int

	// if any of the sprite's draw positions are reached but a reset position
	// signal has been scheduled, then we need to delay the start of the
	// sprite's graphics scan. the drawing actually commences when the reset
	// actually takes place (concept shared with missile sprite)
	deferDrawStart bool

	// the player sprite can be stretched to create single, double or quadruple
	// width sprites. it does this by only ticking the graphics scan counter
	// occassionaly (the missile sprite achieves stretching by a different
	// method).
	graphicsScanFilter int

	// notes whether a reset has just occurred on the last cycle -- used to
	// delay the drawing of the sprite in certain circumstances
	resetTriggered bool
}

func newPlayerSprite(label string, colorClock *polycounter.Polycounter) *playerSprite {
	ps := new(playerSprite)
	ps.sprite = newSprite(label, colorClock, ps.tick)
	return ps
}

// visual pixel tells us where the left-most bit of the graphics register will
// appear. due to the delayed tick of the player sprite the player will appear
// one pixel later in the scanline (or two pixels, depending on the player's
// size register)
//
// the result of this function apes the "Pos#" information in the TIA tab of
// the Stella debugger.
func (ps playerSprite) visualPixel() string {
	// visual pixel is always one pixel later than the hmoved horizontal
	// reset position; or two pixels if the size of the player sprite is double
	// or quadruple sized.
	visPix := ps.currentPixel + 1
	if ps.size == 0x05 || ps.size == 0x07 {
		visPix++
	}

	// adjust for screen boundary
	if visPix >= 160 {
		visPix -= 160
	}

	return fmt.Sprintf("%d", visPix)
}

// realPixel is a variant on visualPixel() that takes into account where the
// first on bit is in the graphics register. returns the string "invisible" if
// no bits are set in the register.
func (ps playerSprite) realPixel() string {
	// how many screen-pixels does each sprite-pixel consume
	pixelWidth := 1
	if ps.size == 0x05 || ps.size == 0x07 {
		pixelWidth = 2
	}

	// select which graphics register to use
	gfxData := ps.gfxDataA
	if ps.verticalDelay {
		gfxData = ps.gfxDataB
	}

	// reverse the bits if necessary
	if ps.reflected {
		gfxData = bits.Reverse8(gfxData)
	}

	visPix := -1

	// find first on bit in gfxData; note that we're looping from 2 to 9 (a
	// range of eight) because we want the multiplier i to take into account
	// the first always-dead sprite pixel (see visualPixel() commentary)
	m := uint8(0x80)
	for i := 2; i <= 9; i++ {
		if gfxData&m == m {
			// when we've found it, move visual pixel the appropriate number of
			// places to the right (by adding multiplies of pixelWidth)
			visPix = ps.currentPixel + (i * pixelWidth)
			break // for loop
		}
		m >>= 1
	}

	// there are no on bits in the gxfData
	if visPix == -1 {
		return "invisible"
	}

	// adjust for screen boundary
	if visPix >= 160 {
		visPix -= 160
	}

	return fmt.Sprintf("%d", visPix)
}

// MachineInfo returns the player sprite information in terse format
func (ps playerSprite) MachineInfoTerse() string {
	return fmt.Sprintf("%s (vis pix=%s)", ps.sprite.MachineInfoTerse(), ps.visualPixel())
}

// MachineInfo returns the player sprite information in verbose format
func (ps playerSprite) MachineInfo() string {
	s := strings.Builder{}

	s.WriteString(fmt.Sprintf("   visual pixel: %s", ps.visualPixel()))
	if ps.horizMovementLatch {
		s.WriteString(fmt.Sprintf(" *\n"))
	} else {
		s.WriteString(fmt.Sprintf("\n"))
	}
	s.WriteString(fmt.Sprintf("   color: %d\n", ps.color))
	s.WriteString(fmt.Sprintf("   size: %03b [", ps.size))
	switch ps.size {
	case 0:
		s.WriteString("one copy")
	case 1:
		s.WriteString("two copies (close)")
	case 2:
		s.WriteString("two copies (medium)")
	case 3:
		s.WriteString("three copies (close)")
	case 4:
		s.WriteString("two copies (wide)")
	case 5:
		s.WriteString("double size")
	case 6:
		s.WriteString("three copies (medium)")
	case 7:
		s.WriteString("quad size")
	}
	s.WriteString("]\n")
	s.WriteString("   trigger list: ")
	if len(ps.triggerList) > 0 {
		for i := 0; i < len(ps.triggerList); i++ {
			// additional pixels when the graphics scan is triggered (NOT the
			// visual pixel)
			s.WriteString(fmt.Sprintf("%d ", (ps.triggerList[i]*(polycounter.MaxPhase+1))+ps.currentPixel))
		}
		s.WriteString(fmt.Sprintf(" %v\n", ps.triggerList))
	} else {
		s.WriteString("none\n")
	}
	s.WriteString(fmt.Sprintf("   reflected: %v\n", ps.reflected))
	s.WriteString(fmt.Sprintf("   vert delay: %v\n", ps.verticalDelay))
	s.WriteString(fmt.Sprintf("   gfx: %08b\n", ps.gfxDataA))
	s.WriteString(fmt.Sprintf("   delayed gfx: %08b\n", ps.gfxDataB))

	return fmt.Sprintf("%s%s", ps.sprite.MachineInfo(), s.String())
}

// tick moves the counters along for the player sprite
func (ps *playerSprite) tick() {
	// position
	if ps.checkForGfxStart(ps.triggerList) {
		// this is a wierd one. if a reset has just occured then we delay the
		// start of the drawing of the sprite, unless the position of the
		// sprite has been moved with HMOVE.
		//
		// the first part of the condition was tuned with the help of the
		// "player testcard" roms. the additional condition, regarding the
		// effects of HMOVE, was added after seeing errors in Mott's test code,
		// "Games that do bad things to HMOVE...". not at all sure this is an
		// accurate solution.
		//
		// (concept shared with missile sprite)
		if ps.resetFuture != nil && !ps.resetTriggered && ps.resetPixel == ps.currentPixel {
			ps.deferDrawStart = true
		} else {
			ps.startDrawing()
		}

		if ps.size == 0x05 {
			ps.graphicsScanFilter = 1
		} else if ps.size == 0x07 {
			ps.graphicsScanFilter = 3
		}

	} else {
		// if player.position.tick() has not caused the position counter to
		// cycle then progress draw signal according to color clock phase and
		// ps.size. for ps.size and 0b101 and 0b111, pixels are smeared over
		// additional cycles in order to create the double and quadruple sized
		// sprites
		if ps.size == 0x05 {
			if ps.graphicsScanFilter%2 == 0 {
				ps.tickGraphicsScan()
			}
		} else if ps.size == 0x07 {
			if ps.graphicsScanFilter%4 == 0 {
				ps.tickGraphicsScan()
			}
		} else {
			ps.tickGraphicsScan()
		}

		if !ps.deferDrawStart {
			ps.graphicsScanFilter++
		}
	}

	ps.resetTriggered = false
}

// pixel returns the color of the player at the current time.  returns
// (false, col) if no pixel is to be seen; and (true, col) if there is
func (ps *playerSprite) pixel() (bool, uint8) {
	// select which graphics register to use
	gfxData := ps.gfxDataA
	if ps.verticalDelay {
		gfxData = ps.gfxDataB
	}

	// reverse the bits if necessary
	if ps.reflected {
		gfxData = bits.Reverse8(gfxData)
	}

	// player sprites are unusual in that the first tick of the draw signal is
	// discounted
	if ps.isDrawing() && ps.graphicsScanCounter > 0 {
		if gfxData>>(uint8(ps.graphicsScanMax)-uint8(ps.graphicsScanCounter))&0x01 == 0x01 {
			return true, ps.color
		}
	}

	// always return player color because when in "scoremode", the playfield
	// wants to know what the color should be
	return false, ps.color
}

func (ps *playerSprite) scheduleReset(onFutureWrite *future.Group) {
	ps.resetTriggered = true
	ps.resetFuture = onFutureWrite.Schedule(delay.ResetPlayer, func() {
		ps.resetFuture = nil
		ps.resetTriggered = false
		ps.resetPosition()
		if ps.deferDrawStart {
			ps.startDrawing()
			ps.deferDrawStart = false
		}
	}, fmt.Sprintf("%s resetting", ps.label))
}

func (ps *playerSprite) scheduleWrite(data uint8, onFutureWrite *future.Group) {
	onFutureWrite.Schedule(delay.WritePlayer, func() {
		ps.otherPlayer.gfxDataB = ps.otherPlayer.gfxDataA
	}, fmt.Sprintf("%s updating vdel gfx register", ps.otherPlayer.label))

	onFutureWrite.Schedule(delay.WritePlayer, func() {
		ps.gfxDataA = data
	}, fmt.Sprintf("%s writing data", ps.label))
}

func (ps *playerSprite) scheduleVerticalDelay(vdelay bool, onFutureWrite *future.Group) {
	label := "enabling vertical delay"
	if !vdelay {
		label = "disabling vertical delay"
	}

	onFutureWrite.Schedule(delay.SetVDELP, func() {
		ps.verticalDelay = vdelay
	}, fmt.Sprintf("%s %s", ps.label, label))
}
