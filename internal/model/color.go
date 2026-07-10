package model

import (
	"strconv"
	"strings"
)

// ansi16 is the RGB of the 16 standard ANSI terminal colors, in palette-index
// order (0=black … 15=bright white). The order matches the terminal's own
// palette entries and the tcell named colors the UI maps each index to, so a
// nearest-color match here renders as that palette entry. LazyPlanner draws only
// from these 16 so it inherits the terminal theme (see main.md).
var ansi16 = [16][3]int{
	0:  {0x00, 0x00, 0x00}, // black
	1:  {0x80, 0x00, 0x00}, // maroon (red)
	2:  {0x00, 0x80, 0x00}, // green
	3:  {0x80, 0x80, 0x00}, // olive (yellow)
	4:  {0x00, 0x00, 0x80}, // navy (blue)
	5:  {0x80, 0x00, 0x80}, // purple (magenta)
	6:  {0x00, 0x80, 0x80}, // teal (cyan)
	7:  {0xc0, 0xc0, 0xc0}, // silver (white)
	8:  {0x80, 0x80, 0x80}, // gray (bright black)
	9:  {0xff, 0x00, 0x00}, // red (bright)
	10: {0x00, 0xff, 0x00}, // lime (bright green)
	11: {0xff, 0xff, 0x00}, // yellow (bright)
	12: {0x00, 0x00, 0xff}, // blue (bright)
	13: {0xff, 0x00, 0xff}, // fuchsia (bright magenta)
	14: {0x00, 0xff, 0xff}, // aqua (bright cyan)
	15: {0xff, 0xff, 0xff}, // white (bright)
}

// NearestANSI16 maps a hex color to the closest of the 16 standard ANSI terminal
// colors, returning its palette index (0–15). It accepts "#rrggbb" or
// "#rrggbbaa" (the Apple calendar-color forms; a leading '#' is optional and any
// alpha channel is ignored). ok is false when s is not a valid hex color, so the
// caller can keep its default color.
func NearestANSI16(s string) (index int, ok bool) {
	r, g, b, ok := parseHexRGB(s)
	if !ok {
		return 0, false
	}
	best, bestDist := 0, 1<<31-1
	for i, c := range ansi16 {
		dr, dg, db := r-c[0], g-c[1], b-c[2]
		dist := dr*dr + dg*dg + db*db
		if dist < bestDist {
			best, bestDist = i, dist
		}
	}
	return best, true
}

// ANSI16IsDark reports whether ANSI palette color index (0–15) is a dark color,
// using perceived luminance. Callers use it to pick a contrasting foreground
// (white over a dark color, black over a light one) when the color is a fill.
func ANSI16IsDark(index int) bool {
	if index < 0 || index >= len(ansi16) {
		return false
	}
	c := ansi16[index]
	// Rec. 601 perceived luminance; midpoint threshold.
	lum := (299*c[0] + 587*c[1] + 114*c[2]) / 1000
	return lum < 128
}

// Luminance returns the Rec. 601 perceived luminance (0–255) of an RGB color.
func Luminance(r, g, b int) int {
	return (299*r + 587*g + 114*b) / 1000
}

// ParseHexColor parses an "#rrggbb"/"#rrggbbaa" color (the Apple calendar-color
// forms; leading '#' optional, alpha ignored) into 0–255 components. ok is false
// when s is not a valid hex color, so the caller can keep its default.
func ParseHexColor(s string) (r, g, b int, ok bool) {
	return parseHexRGB(s)
}

// minReadableLum is the luminance floor ReadableFg lifts foreground colors to.
// Item colors are drawn as foreground text on the terminal's *default*
// background, which the app can't read; assuming the common dark-terminal case,
// a very dark server color (e.g. navy) would be nearly invisible, so it is
// lightened until it clears this floor. Fills, which carry their own contrasting
// text, keep the exact color.
const minReadableLum = 96

// ReadableFg lightens a color toward white until its luminance clears
// minReadableLum, so it stays legible as foreground text on a dark background. A
// color already at or above the floor is returned unchanged. This assumes a dark
// background (a light-terminal user can force the themed 16-color mode).
func ReadableFg(r, g, b int) (int, int, int) {
	lum := Luminance(r, g, b)
	if lum >= minReadableLum || lum >= 255 {
		return r, g, b
	}
	// Luminance is linear under a blend toward white, so the blend fraction t
	// that reaches the floor exactly is (floor-lum)/(255-lum).
	t := float64(minReadableLum-lum) / float64(255-lum)
	lift := func(c int) int { return c + int(t*float64(255-c)+0.5) }
	return lift(r), lift(g), lift(b)
}

// parseHexRGB parses "#rrggbb" / "#rrggbbaa" (with or without '#', alpha ignored)
// into 0–255 components.
func parseHexRGB(s string) (r, g, b int, ok bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 && len(s) != 8 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s[:6], 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(v>>16) & 0xff, int(v>>8) & 0xff, int(v) & 0xff, true
}
