package sdltv

import (
	"gopher2600/television"

	"github.com/veandco/go-sdl2/sdl"
)

// guiLoop listens for SDL events and is run concurrently. critical sections
// protected by tv.guiLoopLock
func (tv *SDLTV) guiLoop() {
	for {
		ev := sdl.WaitEvent()
		switch ev := ev.(type) {

		// close window
		case *sdl.QuitEvent:
			tv.RequestSetAttr(television.ReqSetVisibility, false)

		case *sdl.KeyboardEvent:
			if ev.Type == sdl.KEYDOWN {
				switch ev.Keysym.Sym {
				case sdl.K_BACKQUOTE:
					tv.guiLoopLock.Lock()
					tv.scr.toggleMasking()
					tv.guiLoopLock.Unlock()

					// TODO: this doesn't work properly because we're in a
					// different goroutine than the one in which we intialised
					// the SDL library.
				}
			}

		case *sdl.MouseButtonEvent:
			if ev.Type == sdl.MOUSEBUTTONDOWN {
				switch ev.Button {

				case sdl.BUTTON_LEFT:
					tv.onMouseButtonLeft.dispatch()

				case sdl.BUTTON_RIGHT:
					sx, sy := tv.scr.renderer.GetScale()

					tv.guiLoopLock.Lock()
					// convert X pixel value to horizpos equivalent
					// the opposite of pixelX() and also the scalining applied
					// by the SDL renderer
					if tv.scr.unmasked {
						tv.mouseX = int(float32(ev.X)/sx) - tv.Spec.ClocksPerHblank
					} else {
						tv.mouseX = int(float32(ev.X) / sx)
					}

					// convert Y pixel value to scanline equivalent
					// the opposite of pixelY() and also the scalining applied
					// by the SDL renderer
					if tv.scr.unmasked {
						tv.mouseY = int(float32(ev.Y) / sy)
					} else {
						tv.mouseY = int(float32(ev.Y)/sy) + tv.Spec.ScanlinesPerVBlank + tv.Spec.ScanlinesPerVSync
					}
					tv.guiLoopLock.Unlock()

					tv.onMouseButtonRight.dispatch()
				}
			}

		case *sdl.MouseMotionEvent:
			// TODO: panning of zoomed image

		case *sdl.MouseWheelEvent:
			// TODO: zoom image

		default:
		}
	}
}
