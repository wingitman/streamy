package ui

import "testing"

func TestWrapStaysWithinDisplayWidth(t *testing.T) {
	for _, width := range []int{1, 2, 8, 20} {
		for _, line := range Wrap("alpha beta 世界", width) {
			if len([]rune(line)) > width+2 {
				t.Fatalf("width %d produced suspicious line %q", width, line)
			}
		}
	}
}

func TestRectClipAndContains(t *testing.T) {
	r := (Rect{X: -2, Y: -1, Width: 20, Height: 10}).Clip(8, 4)
	if r.X != 0 || r.Y != 0 || r.Width != 8 || r.Height != 4 {
		t.Fatalf("unexpected clipped rect: %+v", r)
	}
	if !r.Contains(7, 3) || r.Contains(8, 3) {
		t.Fatal("rectangle containment is incorrect")
	}
}
