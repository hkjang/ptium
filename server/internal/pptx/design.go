package pptx

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Design is the resolved design system a slide is composed with. Every value is
// derived from the template's own theme, so the same rules produce a deck that
// looks like the customer's brand rather than like Ptium.
//
// Colour follows the data-visualization discipline: ink tokens carry text,
// marks carry identity, and a categorical order is fixed rather than cycled.
type Design struct {
	// Surfaces.
	Surface       string
	SurfaceRaised string
	Line          string

	// Ink tokens. Text never wears a data colour.
	InkPrimary   string
	InkSecondary string
	InkMuted     string
	OnAccent     string

	// Data colours.
	Accent      string
	Categorical []string
	DeEmphasis  string
	Positive    string
	Negative    string

	// Typography, in hundredths of a point.
	Display int
	Title   int
	Heading int
	Body    int
	Small   int
	Micro   int
	Major   string
	Minor   string

	// Unit is the spacing rhythm in EMU: every gap is a multiple of it.
	Unit int

	// Dark reports whether the surface is dark, which flips a few decisions
	// that cannot be derived from contrast alone.
	Dark bool
}

// Spacing rhythm helpers. Slides are large surfaces, so the base unit is 8pt.
const designUnit = 8 * EMUPerPoint

// NewDesign resolves the design system for a template.
func NewDesign(manifest Manifest) Design {
	theme := manifest.Theme
	// The surface is what a slide actually paints, not whichever theme slot is
	// named "light": a dark template legitimately puts its dark background in
	// lt1, and reading the slot instead of the background inverts every ink.
	surface := dominantBackground(manifest)
	ink := readableInk(surface, theme.Color("lt1"), theme.Color("dk1"))
	if contrastRatio(ink, surface) < 4.5 {
		ink = readableInk(surface, "FFFFFF", "000000")
	}
	dark := relativeLuminance(surface) < 0.4

	design := Design{
		Surface:       surface,
		SurfaceRaised: mixColor(surface, ink, ifElse(dark, 0.10, 0.045)),
		Line:          mixColor(surface, ink, ifElse(dark, 0.24, 0.14)),
		InkPrimary:    ink,
		// Secondary and muted ink fade toward the surface only as far as they
		// can while staying readable. A fixed mix looks right on a light theme
		// and disappears on a dark one.
		InkSecondary: fadeInk(ink, surface, 0.28, 4.5),
		InkMuted:     fadeInk(ink, surface, 0.48, 3.0),
		Accent:       theme.Color("accent1"),
		DeEmphasis:   fadeInk(ink, surface, 0.62, 1.6),
		Display:      4400,
		Title:        3200,
		Heading:      2000,
		Body:         1600,
		Small:        1200,
		Micro:        1000,
		Major:        theme.MajorLatin,
		Minor:        theme.MinorLatin,
		Unit:         designUnit,
		Dark:         dark,
	}
	design.OnAccent = readableInk(design.Accent, surface, ink)
	design.Categorical = categoricalOrder(theme, surface)
	design.Positive, design.Negative = statusColors(theme, dark)
	return design
}

// dominantBackground is the background most of a template's layouts paint.
func dominantBackground(manifest Manifest) string {
	counts := map[string]int{}
	for _, layout := range manifest.Layouts {
		if hexColorPattern.MatchString(layout.Background) {
			counts[layout.Background]++
		}
	}
	best, bestCount := "", 0
	for value, count := range counts {
		// Ties break on the value so the result never depends on map order.
		if count > bestCount || (count == bestCount && value < best) {
			best, bestCount = value, count
		}
	}
	if best != "" {
		return best
	}
	return manifest.Theme.Color("lt1")
}

// Series returns the colour for a categorical slot, counting from zero. Slots
// past the validated order fold into the de-emphasis grey rather than inventing
// a hue, which is what keeps a legend honest.
func (d Design) Series(index int) string {
	if index < 0 || len(d.Categorical) == 0 {
		return d.Accent
	}
	if index >= len(d.Categorical) {
		return d.DeEmphasis
	}
	return d.Categorical[index]
}

// SeriesCap is the number of categorical slots this template can carry.
func (d Design) SeriesCap() int {
	if len(d.Categorical) == 0 {
		return 1
	}
	return len(d.Categorical)
}

// Sequential returns a step of a single-hue ramp, from lightest at step 0 to
// the full accent at the last step. One hue, more-is-darker: the safe default
// for magnitude.
func (d Design) Sequential(step, steps int) string {
	if steps <= 1 {
		return d.Accent
	}
	if step < 0 {
		step = 0
	}
	if step >= steps {
		step = steps - 1
	}
	// Keep the lightest step visible against the surface.
	position := 0.35 + 0.65*float64(step)/float64(steps-1)
	return mixColor(d.Surface, d.Accent, position)
}

// Track is the unfilled part of a meter: a lighter step of the accent's own
// ramp, so state reads across the whole bar.
func (d Design) Track() string { return mixColor(d.Surface, d.Accent, ifElse(d.Dark, 0.22, 0.16)) }

// categoricalOrder builds the fixed categorical order from the theme's accents,
// dropping any slot a reader could not tell apart from one already accepted.
// The check is the same perceptual distance a designer would validate by hand,
// computed instead of eyeballed.
func categoricalOrder(theme Theme, surface string) []string {
	order := make([]string, 0, 6)
	for index := 1; index <= 6; index++ {
		candidate := strings.ToUpper(strings.TrimSpace(theme.Colors[fmt.Sprintf("accent%d", index)]))
		if !hexColorPattern.MatchString(candidate) {
			continue
		}
		// A hue that vanishes into the surface cannot carry identity, a near-grey
		// slot collides with the de-emphasis colour reserved for context, and a
		// slot that reads as one already accepted — to any eye, including one that
		// cannot separate red from green — is not a second series.
		if rejectionReason(candidate, surface, order) != "" {
			continue
		}
		order = append(order, candidate)
	}
	if len(order) == 0 {
		order = append(order, theme.Color("accent1"))
	}
	return order
}

// statusColors picks green/red steps that stay legible on the surface. Status
// colours are reserved: they never stand in for a categorical slot.
func statusColors(theme Theme, dark bool) (positive, negative string) {
	positive, negative = "1F9D55", "D64545"
	if dark {
		positive, negative = "3FBF7F", "F07070"
	}
	// Prefer a theme accent when one is unambiguously green or red, so a
	// customer's brand palette is not overridden for no reason.
	for index := 1; index <= 6; index++ {
		value := strings.ToUpper(strings.TrimSpace(theme.Colors[fmt.Sprintf("accent%d", index)]))
		if !hexColorPattern.MatchString(value) {
			continue
		}
		hue, saturation, _ := hsl(value)
		if saturation < 0.25 {
			continue
		}
		switch {
		case hue >= 95 && hue <= 165:
			positive = value
		case hue >= 345 || hue <= 12:
			negative = value
		}
	}
	return positive, negative
}

// fadeInk mixes ink toward the surface by at most amount, backing off until the
// result clears a contrast floor.
func fadeInk(ink, surface string, amount, minimumContrast float64) string {
	for step := amount; step > 0.02; step -= 0.04 {
		candidate := mixColor(ink, surface, step)
		if contrastRatio(candidate, surface) >= minimumContrast {
			return candidate
		}
	}
	return strings.ToUpper(strings.TrimPrefix(ink, "#"))
}

// readableInk returns whichever of two inks contrasts better with a fill.
func readableInk(fill, light, dark string) string {
	if contrastRatio(fill, light) >= contrastRatio(fill, dark) {
		return light
	}
	return dark
}

// --- colour maths -----------------------------------------------------------

func parseHex(value string) (float64, float64, float64) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return 0, 0, 0
	}
	component := func(offset int) float64 {
		parsed, err := strconv.ParseUint(value[offset:offset+2], 16, 8)
		if err != nil {
			return 0
		}
		return float64(parsed) / 255
	}
	return component(0), component(2), component(4)
}

func formatHex(r, g, b float64) string {
	clamp := func(value float64) int {
		scaled := int(math.Round(value * 255))
		if scaled < 0 {
			return 0
		}
		if scaled > 255 {
			return 255
		}
		return scaled
	}
	return fmt.Sprintf("%02X%02X%02X", clamp(r), clamp(g), clamp(b))
}

// mixColor blends toward another colour in linear light, which keeps midpoints
// from muddying the way an sRGB average does.
func mixColor(from, to string, amount float64) string {
	if amount <= 0 {
		return strings.ToUpper(strings.TrimPrefix(from, "#"))
	}
	if amount >= 1 {
		return strings.ToUpper(strings.TrimPrefix(to, "#"))
	}
	fr, fg, fb := parseHex(from)
	tr, tg, tb := parseHex(to)
	blend := func(a, b float64) float64 {
		return toSRGB(toLinear(a)*(1-amount) + toLinear(b)*amount)
	}
	return formatHex(blend(fr, tr), blend(fg, tg), blend(fb, tb))
}

func toLinear(channel float64) float64 {
	if channel <= 0.04045 {
		return channel / 12.92
	}
	return math.Pow((channel+0.055)/1.055, 2.4)
}

func toSRGB(channel float64) float64 {
	if channel <= 0.0031308 {
		return channel * 12.92
	}
	return 1.055*math.Pow(channel, 1/2.4) - 0.055
}

func relativeLuminance(value string) float64 {
	r, g, b := parseHex(value)
	return 0.2126*toLinear(r) + 0.7152*toLinear(g) + 0.0722*toLinear(b)
}

func contrastRatio(a, b string) float64 {
	first, second := relativeLuminance(a)+0.05, relativeLuminance(b)+0.05
	if first < second {
		first, second = second, first
	}
	return first / second
}

// oklab converts sRGB to OKLab, the space perceptual distance is measured in.
func oklab(value string) (l, a, b float64) {
	r, g, blue := parseHex(value)
	lr, lg, lb := toLinear(r), toLinear(g), toLinear(blue)
	longM := math.Cbrt(0.4122214708*lr + 0.5363325363*lg + 0.0514459929*lb)
	mediumM := math.Cbrt(0.2119034982*lr + 0.6806995451*lg + 0.1073969566*lb)
	shortM := math.Cbrt(0.0883024619*lr + 0.2817188376*lg + 0.6299787005*lb)
	return 0.2104542553*longM + 0.7936177850*mediumM - 0.0040720468*shortM,
		1.9779984951*longM - 2.4285922050*mediumM + 0.4505937099*shortM,
		0.0259040371*longM + 0.7827717662*mediumM - 0.8086757660*shortM
}

// colorDistance is the OKLab distance scaled by 100, matching the thresholds
// design guidance is written against.
func colorDistance(first, second string) float64 {
	l1, a1, b1 := oklab(first)
	l2, a2, b2 := oklab(second)
	return 100 * math.Sqrt((l1-l2)*(l1-l2)+(a1-a2)*(a1-a2)+(b1-b2)*(b1-b2))
}

// chroma is the OKLab colourfulness of a value: how far it sits from grey.
func chroma(value string) float64 {
	_, a, b := oklab(value)
	return math.Sqrt(a*a + b*b)
}

func hsl(value string) (hue, saturation, lightness float64) {
	r, g, b := parseHex(value)
	maximum := math.Max(r, math.Max(g, b))
	minimum := math.Min(r, math.Min(g, b))
	lightness = (maximum + minimum) / 2
	if maximum == minimum {
		return 0, 0, lightness
	}
	delta := maximum - minimum
	if lightness > 0.5 {
		saturation = delta / (2 - maximum - minimum)
	} else {
		saturation = delta / (maximum + minimum)
	}
	switch maximum {
	case r:
		hue = math.Mod((g-b)/delta, 6)
	case g:
		hue = (b-r)/delta + 2
	default:
		hue = (r-g)/delta + 4
	}
	hue *= 60
	if hue < 0 {
		hue += 360
	}
	return hue, saturation, lightness
}

func ifElse(condition bool, whenTrue, whenFalse float64) float64 {
	if condition {
		return whenTrue
	}
	return whenFalse
}
