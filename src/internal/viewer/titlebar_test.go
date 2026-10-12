package viewer

import (
	"testing"

	"mdview/internal/renderer"
)

func TestColorRef(t *testing.T) {
	cases := map[uint32]uint32{
		0x112233: 0x332211,
		0xff0000: 0x0000ff,
		0x00ff00: 0x00ff00,
		0x0000ff: 0xff0000,
	}
	for in, want := range cases {
		if got := colorRef(renderer.RGB(in)); got != want {
			t.Errorf("colorRef(%06x) = %06x, want %06x", in, got, want)
		}
	}
}
