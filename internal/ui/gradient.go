package ui

// The focused pane's border gradient is ported from tuios, internal/app/border_gradient.go and
// internal/overlay/oklab.go (github.com/Gaurav-Gosain/tuios): the frame is drawn in one color, then a pass
// recolors the focused frame's perimeter cells that carry that color, in gradLevels steps so a run of
// cells shares one SGR; the steps are blended once per palette in OKLab; the 16-color palette has no
// gradient, its slots do not blend.
// MIT License, Copyright (c) 2025 Gaurav Gosain.

import (
	"image/color"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// gradLevels is how many colors the gradient steps through.
const gradLevels = 8

// gradPre is the SGR that starts each gradient step, focusPre the one that starts focusBorder, both set by
// usePalette; gradPre is empty for a palette without a gradient.
var (
	gradPre  []string
	focusPre string
	sgrReset = "\x1b[m"
)

// lab is a color in OKLab, where equal steps of a blend look like equal steps.
type lab struct{ l, a, b float64 }

func linearize(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func delinearize(v float64) uint8 {
	v = min(max(v, 0), 1)
	if v <= 0.0031308 {
		v *= 12.92
	} else {
		v = 1.055*math.Pow(v, 1/2.4) - 0.055
	}
	return uint8(math.Round(v * 255))
}

// toLab converts c to OKLab.
func toLab(c color.Color) lab {
	r16, g16, b16, _ := c.RGBA()
	r, g, b := linearize(float64(r16>>8)/255), linearize(float64(g16>>8)/255), linearize(float64(b16>>8)/255)
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return lab{
		l: 0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		a: 1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		b: 0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

// rgba converts back to 8-bit sRGB, clamping a channel outside the gamut.
func (c lab) rgba() color.RGBA {
	l := c.l + 0.3963377774*c.a + 0.2158037573*c.b
	m := c.l - 0.1055613458*c.a - 0.0638541728*c.b
	s := c.l - 0.0894841775*c.a - 1.2914855480*c.b
	l, m, s = l*l*l, m*m*m, s*s*s
	return color.RGBA{
		R: delinearize(+4.0767416621*l - 3.3077115913*m + 0.2309699292*s),
		G: delinearize(-1.2684380046*l + 2.6097574011*m - 0.3413193965*s),
		B: delinearize(-0.0041960863*l - 0.7034186147*m + 1.7076147010*s),
		A: 0xFF,
	}
}

// mix blends a toward b by t in 0..1, in OKLab.
func mix(a, b color.Color, t float64) color.RGBA {
	x, y := toLab(a), toLab(b)
	return lab{x.l + (y.l-x.l)*t, x.a + (y.a-x.a)*t, x.b + (y.b-x.b)*t}.rgba()
}

// gradRect is the focused pane's frame, corners included, in screen cells.
type gradRect struct{ x0, y0, x1, y1 int }

// level is the step of cell (x, y): 45 degrees, corner to corner whatever the frame's shape, and -1 for a
// cell off the perimeter, which keeps its ink.
func (r gradRect) level(x, y int) int {
	if r.x1 <= r.x0 || r.y1 <= r.y0 || (x != r.x0 && x != r.x1 && y != r.y0 && y != r.y1) {
		return -1
	}
	t := (float64(x-r.x0)/float64(r.x1-r.x0) + float64(y-r.y0)/float64(r.y1-r.y0)) / 2
	return min(max(int(math.Round(t*(gradLevels-1))), 0), gradLevels-1)
}

// litGlyph is the lit border glyph s on cell (x, y): its gradient step, or focusBorder where there is none.
func litGlyph(r gradRect, x, y int, s string) string {
	if l := r.level(x, y); l >= 0 && len(gradPre) > 0 {
		return gradPre[l] + s + sgrReset
	}
	return borderGlyph(true, s)
}

// withGradient recolors the focusBorder runs of r's top and bottom rows. The rows between take their
// steps as they are built (litGlyph): finding a border column here would measure every chat row.
func withGradient(rows []string, r gradRect) {
	if len(gradPre) == 0 {
		return
	}
	for _, y := range []int{r.y0, r.y1} {
		if y >= len(rows) {
			continue
		}
		row, col := rows[y], 0
		var b strings.Builder
		for {
			i := strings.Index(row, focusPre)
			j := strings.Index(row[max(i, 0)+len(focusPre):], sgrReset)
			if i < 0 || j < 0 {
				b.WriteString(row)
				break
			}
			col += ansi.StringWidth(row[:i])
			text := row[i+len(focusPre) : i+len(focusPre)+j]
			b.WriteString(row[:i])
			b.WriteString(gradRun(r, col, y, text))
			col += ansi.StringWidth(text)
			row = row[i+len(focusPre)+j+len(sgrReset):]
		}
		rows[y] = b.String()
	}
}

// gradRun is text, drawn from column x of row y, in its cells' steps: one SGR per run of a step.
func gradRun(r gradRect, x, y int, text string) string {
	var b strings.Builder
	last, run := -2, ""
	flush := func() {
		if run == "" {
			return
		}
		pre := focusPre
		if last >= 0 {
			pre = gradPre[last]
		}
		b.WriteString(pre + run + sgrReset)
	}
	for _, c := range text { // border glyphs are one cell each
		if l := r.level(x, y); l != last {
			flush()
			last, run = l, ""
		}
		run += string(c)
		x++
	}
	flush()
	return b.String()
}
