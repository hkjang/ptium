package pptx

import (
	"math"
	"strings"
	"testing"
)

func TestSimulateVisionCollapsesRedAndGreen(t *testing.T) {
	red, green := "D62728", "2CA02C"
	if colorDistance(red, green) < 25 {
		t.Fatalf("the fixture colours are not far apart to begin with: %.1f", colorDistance(red, green))
	}
	// What a deficiency removes is the hue difference. Lightness survives, which
	// is why a protanope can still tell this particular pair apart and a
	// deuteranope cannot — so the hue axis is what the simulation must collapse.
	hueGap := func(first, second string, kind visionKind) float64 {
		_, a1, b1 := oklab(simulateVision(first, kind))
		_, a2, b2 := oklab(simulateVision(second, kind))
		return 100 * math.Sqrt((a1-a2)*(a1-a2)+(b1-b2)*(b1-b2))
	}
	if before := hueGap(red, green, visionNormal); before < 20 {
		t.Fatalf("the fixture hues are not far apart to begin with: %.1f", before)
	}
	for _, kind := range []visionKind{visionProtan, visionDeuteran} {
		if gap := hueGap(red, green, kind); gap > 8 {
			t.Fatalf("%s should collapse the hue difference, got %.1f", kind, gap)
		}
	}
	// Overall, the pair must be reported as failing: a deuteran eye sees one series.
	if _, _, ok := separates(red, green); ok {
		t.Fatal("red against green must not count as two data colours")
	}
	// A blue against a yellow survives, because that axis is unaffected.
	if _, _, ok := separates("1F77B4", "FFB000"); !ok {
		t.Fatal("blue against amber must remain distinguishable")
	}
	// Simulation is a no-op for a value it cannot read, and for normal vision.
	if got := simulateVision("nonsense", visionProtan); got != "NONSENSE" {
		t.Fatalf("simulateVision(%q) = %q", "nonsense", got)
	}
	if got := simulateVision("#1F77B4", visionNormal); got != "#1F77B4" {
		t.Fatalf("normal vision must not alter a colour: %q", got)
	}
}

func TestSeparatesRejectsAPairNobodyCanTellApart(t *testing.T) {
	if kind, distance, ok := separates("1F77B4", "2079B6"); ok {
		t.Fatalf("two near-identical blues must not separate (%s, ΔE %.1f)", kind, distance)
	} else if kind != visionNormal {
		t.Fatalf("the failure should be reported for normal vision, got %s", kind)
	}
	if kind, _, ok := separates("D62728", "2CA02C"); ok {
		t.Fatal("red against green must be reported as failing")
	} else if kind == visionNormal {
		t.Fatal("red against green separates for normal vision; the failure is a deficiency")
	}
}

func TestAuditThemeExplainsWhatAPaletteSupports(t *testing.T) {
	manifest := Manifest{
		Version: ManifestVersion, SlideWidth: 12192000, SlideHeight: 6858000,
		Theme: Theme{Colors: map[string]string{
			"lt1": "FFFFFF", "dk1": "111111",
			"accent1": "1F77B4", // a usable blue
			"accent2": "D62728", // a usable red
			"accent3": "2CA02C", // a green that collides with the red for a deuteran eye
			"accent4": "F2F2F2", // vanishes into the white surface
			"accent5": "9A9A9A", // grey, reserved for de-emphasis
			"accent6": "not-a-colour",
		}},
		Layouts: []Layout{{ID: "content", Name: "Content", Role: RoleContent, Background: "FFFFFF",
			Placeholders: []Placeholder{{Slot: SlotTitle, Kind: "text"}}}},
	}
	audit := AuditTheme(manifest)
	if audit.Surface != "FFFFFF" {
		t.Fatalf("surface = %s", audit.Surface)
	}
	if audit.InkContrast < 4.5 {
		t.Fatalf("ink contrast = %.2f, which would be unreadable", audit.InkContrast)
	}
	if len(audit.DataColors) != 2 {
		t.Fatalf("data colours = %v, want the blue and the red only", audit.DataColors)
	}
	reasons := map[string]string{}
	for _, rejection := range audit.Rejected {
		reasons[rejection.Slot] = rejection.Reason
	}
	if !strings.Contains(reasons["accent3"], "deuteranopia") && !strings.Contains(reasons["accent3"], "protanopia") {
		t.Fatalf("accent3 should be rejected for colour vision, got %q", reasons["accent3"])
	}
	if !strings.Contains(reasons["accent4"], "background") {
		t.Fatalf("accent4 should be rejected against the background, got %q", reasons["accent4"])
	}
	if !strings.Contains(reasons["accent5"], "grey") {
		t.Fatalf("accent5 should be rejected as grey, got %q", reasons["accent5"])
	}
	if reasons["accent6"] == "" {
		t.Fatal("an unreadable colour should be reported rather than ignored")
	}
	// The audit and the renderer must agree; a report about colours the charts do
	// not use would be worse than none.
	if strings.Join(audit.DataColors, ",") != strings.Join(NewDesign(manifest).Categorical, ",") {
		t.Fatalf("audit %v disagrees with the design %v", audit.DataColors, NewDesign(manifest).Categorical)
	}
}

func TestBuiltinDesignsStillSeparateForEveryEye(t *testing.T) {
	for _, key := range BuiltinDesignKeys() {
		design := LookupBuiltinDesign(key)
		data, err := BuiltinTemplate(key)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		_, manifest, err := AnalyzeBytes(data)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		audit := AuditTheme(manifest)
		if len(audit.DataColors) < 3 {
			t.Fatalf("%s (%s) offers only %d data colours: %v; rejected %+v",
				key, design.Name, len(audit.DataColors), audit.DataColors, audit.Rejected)
		}
		if audit.InkContrast < 4.5 {
			t.Fatalf("%s: body text contrast is %.2f", key, audit.InkContrast)
		}
	}
}
