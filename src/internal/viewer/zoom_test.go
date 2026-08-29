package viewer

import "testing"

func TestFitScaleNeverUpscales(t *testing.T) {
	// Image smaller than the viewport: fit must not exceed 100%.
	if s := fitScale(100, 50, 800, 600); s != 1 {
		t.Errorf("fitScale(small image) = %v, want 1", s)
	}
}

func TestFitScaleShrinksProportionally(t *testing.T) {
	// 2000x1000 into an 800x800 box: width is the binding constraint (0.4),
	// height would allow 0.8, so the smaller ratio wins.
	got := fitScale(2000, 1000, 800, 800)
	want := 0.4
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("fitScale(2000x1000 into 800x800) = %v, want %v", got, want)
	}
}

func TestLayoutRecomputesFitOnResize(t *testing.T) {
	z := newZoomState(2000, 1000)
	z.Layout(800, 800)
	if got := z.Percent(); got != 40 {
		t.Fatalf("after first layout, Percent() = %v, want 40", got)
	}
	z.Layout(400, 400)
	if got := z.Percent(); got != 20 {
		t.Errorf("after resize while still in fit mode, Percent() = %v, want 20", got)
	}
}

func TestManualZoomStaysFixedAcrossResize(t *testing.T) {
	z := newZoomState(2000, 1000)
	z.Layout(800, 800)
	z.ZoomAt(1, 400, 400, 800, 800) // zoom in once, centered
	after := z.Percent()

	z.Layout(500, 500) // resize while manually zoomed
	if got := z.Percent(); got != after {
		t.Errorf("manual zoom changed across resize: %v -> %v, want it to stick", after, got)
	}
}

func TestResetReturnsToFit(t *testing.T) {
	z := newZoomState(2000, 1000)
	z.Layout(800, 800)
	z.ZoomAt(3, 100, 100, 800, 800)
	if z.fit {
		t.Fatal("expected fit=false after manual zoom")
	}
	z.Reset()
	if !z.fit {
		t.Fatal("Reset() did not switch back to fit mode")
	}
	z.Layout(800, 800)
	if got := z.Percent(); got != 40 {
		t.Errorf("after Reset, Percent() = %v, want 40 (back to fit)", got)
	}
}

func TestZoomAtKeepsCursorPointFixed(t *testing.T) {
	// Viewport (500x500) is smaller than the image at the starting scale, on
	// both axes, both before and after zooming in one notch — so clampPan
	// has room to pan rather than falling back to centering, and the point
	// under the cursor should stay put.
	z := &zoomState{imgW: 1000, imgH: 1000, fit: false, scale: 1}
	imgX := z.panX + 250/z.scale
	imgY := z.panY + 250/z.scale
	z.ZoomAt(1, 250, 250, 500, 500)
	gotX := z.panX + 250/z.scale
	gotY := z.panY + 250/z.scale
	if diff := gotX - imgX; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("image point under cursor drifted on X: got %v, want %v", gotX, imgX)
	}
	if diff := gotY - imgY; diff > 1e-6 || diff < -1e-6 {
		t.Errorf("image point under cursor drifted on Y: got %v, want %v", gotY, imgY)
	}
}

func TestZoomAtRespectsBounds(t *testing.T) {
	z := &zoomState{imgW: 1000, imgH: 1000, fit: false, scale: 1}
	for i := 0; i < 200; i++ {
		z.ZoomAt(-1, 0, 0, 2000, 2000)
	}
	if z.scale < zoomMin {
		t.Errorf("scale %v went below zoomMin %v", z.scale, zoomMin)
	}
	z2 := &zoomState{imgW: 1000, imgH: 1000, fit: false, scale: 1}
	for i := 0; i < 200; i++ {
		z2.ZoomAt(1, 0, 0, 2000, 2000)
	}
	if z2.scale > zoomMax {
		t.Errorf("scale %v went above zoomMax %v", z2.scale, zoomMax)
	}
}

func TestPanClampedWithinImageBounds(t *testing.T) {
	z := &zoomState{imgW: 1000, imgH: 1000, fit: false, scale: 2} // viewport shows 500x500 image px
	z.PanBy(1e9, 1e9, 400, 400)                                   // drag far past the top-left edge
	if z.panX != 0 || z.panY != 0 {
		t.Errorf("pan not clamped at top-left: got (%v, %v), want (0, 0)", z.panX, z.panY)
	}
	z.PanBy(-1e9, -1e9, 400, 400) // drag far past the bottom-right edge
	wantX := 1000.0 - 400.0/2
	wantY := 1000.0 - 400.0/2
	if z.panX != wantX || z.panY != wantY {
		t.Errorf("pan not clamped at bottom-right: got (%v, %v), want (%v, %v)", z.panX, z.panY, wantX, wantY)
	}
}

func TestPanCentersWhenImageSmallerThanViewport(t *testing.T) {
	z := &zoomState{imgW: 100, imgH: 100, fit: false, scale: 1}
	z.clampPan(400, 400) // viewport (in image px) is larger than the image on both axes
	want := -(400.0 - 100.0) / 2
	if z.panX != want || z.panY != want {
		t.Errorf("pan = (%v, %v), want centered at (%v, %v)", z.panX, z.panY, want, want)
	}
}
