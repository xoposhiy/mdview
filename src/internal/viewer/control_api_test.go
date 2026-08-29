package viewer

import (
	"image"
	"image/color"
	"testing"
	"time"
)

func TestLookupKey(t *testing.T) {
	tests := []struct {
		name   string
		wantVK int
		wantJS string
		wantOK bool
	}{
		{"right", vkRight, "ArrowRight", true},
		{"RIGHT", vkRight, "ArrowRight", true},
		{" esc ", vkEscape, "Escape", true},
		{"backspace", vkBack, "Backspace", true},
		{"f5", vkF5, "F5", true},
		{"d", 'D', "d", true},
		{"7", '7', "7", true},
		{"?", '?', "?", true},
		{"nope", 0, "", false},
		{"", 0, "", false},
	}
	for _, tt := range tests {
		vk, js, ok := lookupKey(tt.name)
		if ok != tt.wantOK || vk != tt.wantVK || js != tt.wantJS {
			t.Errorf("lookupKey(%q) = %d, %q, %v; want %d, %q, %v",
				tt.name, vk, js, ok, tt.wantVK, tt.wantJS, tt.wantOK)
		}
	}
}

func TestParseMods(t *testing.T) {
	ctrl, alt, shift := parseMods([]string{"Ctrl", " alt "})
	if !ctrl || !alt || shift {
		t.Errorf("parseMods(ctrl,alt) = %v, %v, %v; want true, true, false", ctrl, alt, shift)
	}
	if ctrl, alt, shift := parseMods(nil); ctrl || alt || shift {
		t.Errorf("parseMods(nil) reported a modifier: %v, %v, %v", ctrl, alt, shift)
	}
	if _, _, shift := parseMods([]string{"shift", "bogus"}); !shift {
		t.Error("parseMods did not recognise shift next to an unknown name")
	}
}

func TestJSString(t *testing.T) {
	if got := jsString(`a"b`); got != `"a\"b"` {
		t.Errorf(`jsString('a"b') = %s; want "a\"b"`, got)
	}
}

func TestIsOpaque(t *testing.T) {
	opaque := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for x := 0; x < 2; x++ {
		for y := 0; y < 2; y++ {
			opaque.Set(x, y, color.RGBA{1, 2, 3, 255})
		}
	}
	if !isOpaque(opaque) {
		t.Error("isOpaque said a fully opaque image has transparency")
	}

	transparent := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if isOpaque(transparent) {
		t.Error("isOpaque said an empty (fully transparent) image is opaque")
	}
}

func TestMetricsStore(t *testing.T) {
	m := newMetricsStore()
	m.add(stageRender, 2*time.Millisecond)
	m.add(stageRender, 6*time.Millisecond)
	m.setSize("htmlBytes", 1234)

	snap := m.snapshot()
	got := snap.Stages[stageRender]
	if got.Count != 2 {
		t.Errorf("Count = %d; want 2", got.Count)
	}
	if got.LastMs != 6 {
		t.Errorf("LastMs = %v; want 6", got.LastMs)
	}
	if got.MaxMs != 6 {
		t.Errorf("MaxMs = %v; want 6", got.MaxMs)
	}
	if got.SumMs != 8 {
		t.Errorf("SumMs = %v; want 8", got.SumMs)
	}
	if snap.Sizes["htmlBytes"] != 1234 {
		t.Errorf("Sizes[htmlBytes] = %d; want 1234", snap.Sizes["htmlBytes"])
	}

	// A snapshot must not change when the store keeps recording.
	m.add(stageRender, time.Millisecond)
	if snap.Stages[stageRender].Count != 2 {
		t.Error("snapshot changed after a later add")
	}
}

func TestEvalRegistryDeliver(t *testing.T) {
	r := newEvalRegistry()
	id, ch := r.begin()
	r.deliver(id, `{"a":1}`)
	select {
	case got := <-ch:
		if got != `{"a":1}` {
			t.Errorf("delivered %q", got)
		}
	default:
		t.Fatal("nothing was delivered")
	}

	// An id that is no longer waiting must be dropped, not block or panic.
	r.end(id)
	r.deliver(id, "late")
	r.deliver("never-issued", "stray")
}
