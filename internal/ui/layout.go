package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Rect is a terminal-cell rectangle shared by rendering and mouse handling.
type Rect struct{ X, Y, Width, Height int }

func (r Rect) Contains(x, y int) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.Width && y < r.Y+r.Height
}

func (r Rect) Clip(width, height int) Rect {
	if r.X < 0 {
		r.Width += r.X
		r.X = 0
	}
	if r.Y < 0 {
		r.Height += r.Y
		r.Y = 0
	}
	if r.X > width {
		r.X, r.Width = width, 0
	}
	if r.Y > height {
		r.Y, r.Height = height, 0
	}
	if r.Width > width-r.X {
		r.Width = width - r.X
	}
	if r.Height > height-r.Y {
		r.Height = height - r.Y
	}
	if r.Width < 0 {
		r.Width = 0
	}
	if r.Height < 0 {
		r.Height = 0
	}
	return r
}

// Wrap measures terminal display cells, not bytes or rune count.
func Wrap(text string, width int) []string {
	if width < 1 {
		return nil
	}
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		for ansi.StringWidth(paragraph) > width {
			cut := fitPrefix(paragraph, width)
			if cut == 0 {
				_, size := utf8.DecodeRuneInString(paragraph)
				cut = size
			}
			line := paragraph[:cut]
			if index := strings.LastIndexByte(line, ' '); index > 0 {
				cut = index
				line = paragraph[:cut]
			}
			result = append(result, line)
			paragraph = strings.TrimLeft(paragraph[cut:], " ")
		}
		result = append(result, paragraph)
	}
	return result
}

func fitPrefix(text string, width int) int {
	cut := 0
	for index := range text {
		if ansi.StringWidth(text[:index]) > width {
			break
		}
		cut = index
	}
	if ansi.StringWidth(text) <= width {
		return len(text)
	}
	return cut
}

func TruncateLines(lines []string, width, height int) []string {
	if width < 1 || height < 1 {
		return nil
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = ansi.Truncate(line, width, "")
	}
	return result
}

func JoinLines(lines []string, width, height int) string {
	return strings.Join(TruncateLines(lines, width, height), "\n")
}
