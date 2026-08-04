package pptx

import (
	"fmt"
	"math"
	"strings"
)

// Data colours have to survive the eye that reads them.
//
// Roughly one man in twelve cannot separate red from green, and a chart whose
// series are distinct only to normal vision is unreadable to them. The built-in
// palettes were checked for this before shipping; a customer's own theme never
// was, and it is the theme that decides the colours of every chart Ptium draws.
// So the check moved into the engine, where it applies to any template.

// visionKind is a way of seeing that a palette has to hold up under.
type visionKind string

const (
	visionNormal   visionKind = "normal"
	visionProtan   visionKind = "protanopia"
	visionDeuteran visionKind = "deuteranopia"
)

// cvdSeparation is the smallest OKLab distance (×100) two data colours may have
// under a simulated colour vision deficiency. Eight is the working threshold the
// palette validator uses; below it two series read as one.
const cvdSeparation = 8.0

// normalSeparation is the floor for full-colour vision. A pair closer than this
// is indistinguishable to everyone, not just to some.
const normalSeparation = 15.0

// vienotMatrices are the Viénot–Brettel–Mollon transforms, applied in linear
// light. Each row is a linear combination of the linear R, G, B channels.
var vienotMatrices = map[visionKind][3][3]float64{
	visionProtan: {
		{0.11238, 0.88762, 0.0},
		{0.11238, 0.88762, 0.0},
		{0.00401, -0.00401, 1.0},
	},
	visionDeuteran: {
		{0.29275, 0.70725, 0.0},
		{0.29275, 0.70725, 0.0},
		{-0.02234, 0.02234, 1.0},
	},
}

// simulateVision returns how a colour appears under a given way of seeing.
func simulateVision(hex string, kind visionKind) string {
	matrix, ok := vienotMatrices[kind]
	if !ok {
		return strings.ToUpper(hex)
	}
	hex = strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(hex), "#"))
	if !hexColorPattern.MatchString(hex) {
		return hex
	}
	red, green, blue := parseHex(hex)
	linear := [3]float64{toLinear(red), toLinear(green), toLinear(blue)}
	var result [3]float64
	for row := range result {
		result[row] = matrix[row][0]*linear[0] + matrix[row][1]*linear[1] + matrix[row][2]*linear[2]
	}
	return formatHex(clampChannel(toSRGB(result[0])), clampChannel(toSRGB(result[1])), clampChannel(toSRGB(result[2])))
}

func clampChannel(value float64) float64 {
	return math.Min(math.Max(value, 0), 1)
}

// separates reports whether two colours stay apart under every way of seeing
// that matters, and names the first one where they do not.
func separates(first, second string) (visionKind, float64, bool) {
	if distance := colorDistance(first, second); distance < normalSeparation {
		return visionNormal, distance, false
	}
	for _, kind := range []visionKind{visionProtan, visionDeuteran} {
		distance := colorDistance(simulateVision(first, kind), simulateVision(second, kind))
		if distance < cvdSeparation {
			return kind, distance, false
		}
	}
	return visionNormal, 0, true
}

// ThemeAudit is what a template's own colours can and cannot do. It is reported
// to whoever uploaded the template, because the answer is a property of their
// design rather than of Ptium.
type ThemeAudit struct {
	// Surface is the colour a slide actually paints behind its content.
	Surface string `json:"surface"`
	// Ink is the body text colour chosen against that surface.
	Ink string `json:"ink"`
	// InkContrast is the contrast ratio of ink on surface. Below 4.5 the template
	// itself is hard to read, whatever Ptium writes into it.
	InkContrast float64 `json:"inkContrast"`
	// DataColors is the ordered set of accents usable as data colours.
	DataColors []string `json:"dataColors"`
	// Rejected explains, per accent slot, why it is not used.
	Rejected []ThemeRejection `json:"rejected,omitempty"`
	// SeriesLimit is how many series a chart drawn in this theme may carry.
	SeriesLimit int `json:"seriesLimit"`
}

// ThemeRejection is one accent slot that cannot serve as a data colour.
type ThemeRejection struct {
	Slot   string `json:"slot"`
	Color  string `json:"color"`
	Reason string `json:"reason"`
}

// AuditTheme reports what a template's palette supports.
func AuditTheme(manifest Manifest) ThemeAudit {
	design := NewDesign(manifest)
	audit := ThemeAudit{
		Surface:     design.Surface,
		Ink:         design.InkPrimary,
		InkContrast: math.Round(contrastRatio(design.InkPrimary, design.Surface)*100) / 100,
		DataColors:  design.Categorical,
		SeriesLimit: design.SeriesCap(),
	}
	accepted := make([]string, 0, 6)
	for index := 1; index <= 6; index++ {
		slot := fmt.Sprintf("accent%d", index)
		candidate := strings.ToUpper(strings.TrimSpace(manifest.Theme.Colors[slot]))
		if !hexColorPattern.MatchString(candidate) {
			if candidate != "" {
				audit.Rejected = append(audit.Rejected, ThemeRejection{Slot: slot, Color: candidate,
					Reason: "not a colour Ptium can read"})
			}
			continue
		}
		if reason := rejectionReason(candidate, design.Surface, accepted); reason != "" {
			audit.Rejected = append(audit.Rejected, ThemeRejection{Slot: slot, Color: candidate, Reason: reason})
			continue
		}
		accepted = append(accepted, candidate)
	}
	return audit
}

// rejectionReason explains why a candidate cannot join the data colours, or
// returns an empty string when it can.
func rejectionReason(candidate, surface string, accepted []string) string {
	if colorDistance(candidate, surface) < 12 {
		return "too close to the slide background to carry identity"
	}
	if chroma(candidate) < 0.045 {
		return "too close to grey, which is reserved for de-emphasised context"
	}
	for _, existing := range accepted {
		kind, distance, ok := separates(candidate, existing)
		if ok {
			continue
		}
		if kind == visionNormal {
			return fmt.Sprintf("indistinguishable from %s (ΔE %.0f)", existing, distance)
		}
		return fmt.Sprintf("indistinguishable from %s under %s (ΔE %.0f)", existing, kind, distance)
	}
	return ""
}
