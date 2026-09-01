package app

import "github.com/wingitman/streamy/internal/ui"

type Layout struct {
	Brand   ui.Rect
	Actions map[string]ui.Rect
	Items   []ui.Rect
}

// BuildLayout is the single source of truth for visible rectangles and mouse
// hitboxes. Content rows are measured before they are clipped to the viewport.
func BuildLayout(width, height int, styles ui.Styles, items []string, scroll int, logo, hints bool, _ mode) Layout {
	return buildLayout(width, height, styles, items, scroll, max(1, height-8), logo, hints)
}

func buildLayout(width, height int, styles ui.Styles, items []string, scroll, rows int, logo, hints bool) Layout {
	l := Layout{Actions: map[string]ui.Rect{}}
	if width < 1 || height < 1 {
		return l
	}
	brand := ui.Brand(styles)
	brandWidth := len([]rune(brand))
	if brandWidth > width {
		brandWidth = width
	}
	l.Brand = ui.Rect{X: width - brandWidth, Y: 0, Width: brandWidth, Height: 1}
	l.Actions["brand"] = l.Brand
	if width > 10 {
		l.Actions["copy"] = ui.Rect{X: width - 12, Y: max(0, height-3), Width: 12, Height: 1}
		l.Actions["help"] = ui.Rect{X: width - 24, Y: max(0, height-1), Width: 8, Height: 1}
		l.Actions["theme"] = ui.Rect{X: width - 32, Y: max(0, height-1), Width: 8, Height: 1}
		l.Actions["history"] = ui.Rect{X: width - 40, Y: max(0, height-1), Width: 8, Height: 1}
		l.Actions["update"] = ui.Rect{X: width - 48, Y: max(0, height-1), Width: 8, Height: 1}
	}
	rows = max(1, rows)
	start := clamp(scroll, 0, max(0, len(items)-rows))
	end := min(len(items), start+rows)
	for i := start; i < end; i++ {
		l.Items = append(l.Items, ui.Rect{X: 0, Y: 2 + i - start, Width: width, Height: 1})
	}
	return l
}
