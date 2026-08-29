package viewer

import "math"

// zoomMin and zoomMax bound how far the user can zoom an image in and out,
// as a fraction of the image's natural size (1.0 = 100%).
const (
	zoomMin = 0.1
	zoomMax = 32.0
	// zoomWheelStep is the multiplicative change applied per wheel notch.
	zoomWheelStep = 1.1
)

// zoomState tracks how an image is displayed inside a viewport: either
// auto-fitted to the viewport (fit == true, scale recomputed on resize) or
// pinned to a manually chosen scale (fit == false, scale sticky across
// resizes) with a pan offset in image pixels.
//
// All coordinates are in image pixels except where noted "viewport".
type zoomState struct {
	imgW, imgH int
	fit        bool
	scale      float64
	// panX, panY is the image-pixel coordinate shown at the viewport's
	// top-left corner.
	panX, panY float64
}

// newZoomState creates a zoomState for an image of the given natural size,
// starting in fit mode.
func newZoomState(imgW, imgH int) *zoomState {
	return &zoomState{imgW: imgW, imgH: imgH, fit: true, scale: 1}
}

// fitScale returns the scale that fits the image into a viewportW x
// viewportH box without upscaling past 100%.
func fitScale(imgW, imgH, viewportW, viewportH int) float64 {
	if imgW <= 0 || imgH <= 0 || viewportW <= 0 || viewportH <= 0 {
		return 1
	}
	s := math.Min(float64(viewportW)/float64(imgW), float64(viewportH)/float64(imgH))
	if s > 1 {
		s = 1
	}
	return s
}

// Reset switches back to auto-fit mode, discarding any manual zoom/pan.
func (z *zoomState) Reset() {
	z.fit = true
	z.panX, z.panY = 0, 0
}

// Layout recomputes scale and clamps pan for the current viewport size. Call
// it after a resize and whenever the viewport size is otherwise known.
//
// In fit mode the scaled image is always <= the viewport on both axes, so
// clampPan's centering branch (viewLen >= imgLen) is what actually centers
// the image; it must run here too, not just for manual zoom.
func (z *zoomState) Layout(viewportW, viewportH int) {
	if z.fit {
		z.scale = fitScale(z.imgW, z.imgH, viewportW, viewportH)
	}
	z.clampPan(viewportW, viewportH)
}

// clampPan keeps the pan offset from showing empty space beyond the image
// edges when the scaled image is larger than the viewport on that axis; on
// an axis where it's smaller, it centers the image instead.
func (z *zoomState) clampPan(viewportW, viewportH int) {
	z.panX = clampAxis(z.panX, float64(z.imgW), float64(viewportW)/z.scale)
	z.panY = clampAxis(z.panY, float64(z.imgH), float64(viewportH)/z.scale)
}

func clampAxis(pan, imgLen, viewLen float64) float64 {
	if viewLen >= imgLen {
		return -(viewLen - imgLen) / 2
	}
	if pan < 0 {
		return 0
	}
	if pan > imgLen-viewLen {
		return imgLen - viewLen
	}
	return pan
}

// ZoomAt applies steps wheel notches (positive = zoom in, negative = zoom
// out; fractional values are allowed for high-resolution wheels/trackpads)
// at the given viewport point, keeping that point fixed on screen, then
// switches out of fit mode.
func (z *zoomState) ZoomAt(steps float64, viewportX, viewportY, viewportW, viewportH int) {
	if steps == 0 {
		return
	}
	// The image point currently under the cursor, in image pixels.
	imgX := z.panX + float64(viewportX)/z.scale
	imgY := z.panY + float64(viewportY)/z.scale

	newScale := z.scale * math.Pow(zoomWheelStep, steps)
	newScale = math.Max(zoomMin, math.Min(zoomMax, newScale))

	z.fit = false
	z.scale = newScale
	z.panX = imgX - float64(viewportX)/z.scale
	z.panY = imgY - float64(viewportY)/z.scale
	z.clampPan(viewportW, viewportH)
}

// PanBy shifts the pan offset by a viewport-pixel delta (e.g. mouse drag)
// and clamps it to the image bounds.
func (z *zoomState) PanBy(dxViewport, dyViewport float64, viewportW, viewportH int) {
	z.panX -= dxViewport / z.scale
	z.panY -= dyViewport / z.scale
	z.clampPan(viewportW, viewportH)
}

// Percent returns the current scale as a rounded percentage for display.
func (z *zoomState) Percent() int {
	return int(math.Round(z.scale * 100))
}
