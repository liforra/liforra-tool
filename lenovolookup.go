package main

import (
	"fmt"
	"math"
	"strings"

	"liforra-tool/internal/lenovospecs"
	"liforra-tool/internal/specsheet"
)

// LookupLenovoSpecs tries Lenovo's own published spec sheet for a model name
// that's already had the brand stripped (e.g. "ThinkPad T470s", not "Lenovo
// ThinkPad T470s" — see specSheet.ts's stripBrand). Returns pre-formatted
// exactly the way the spec-sheet TXT expects (Gewicht/Abmessungen/Ports);
// everything is zero-valued when nothing was found — see
// internal/lenovospecs's package doc for why that's expected and not an
// error worth surfacing to the technician.
func (a *App) LookupLenovoSpecs(model string) specsheet.Fields {
	var f specsheet.Fields
	r := lenovospecs.Lookup(a.ctx, model)
	if r == nil {
		return f
	}
	if r.WeightKG > 0 {
		f.Gewicht = formatWeightKG(r.WeightKG)
	}
	if r.WidthMM > 0 && r.HeightMM > 0 && r.DepthMM > 0 {
		f.Abmessungen = fmt.Sprintf("%d x %d x %d mm", roundMM(r.WidthMM), roundMM(r.HeightMM), roundMM(r.DepthMM))
	}
	f.Ports = specsheet.Ports{
		VGA:         r.Ports.VGA,
		DisplayPort: r.Ports.DisplayPort,
		HDMI:        r.Ports.HDMI,
		DVI:         r.Ports.DVI,
		USB3:        r.Ports.USB3,
		USB2:        r.Ports.USB2,
		Ethernet:    r.Ports.Ethernet,
		Kopfhoerer:  r.Ports.Kopfhoerer,
		Mikrofon:    r.Ports.Mikrofon,
		LineIn:      r.Ports.LineIn,
		PS2:         r.Ports.PS2,
	}
	return f
}

func roundMM(v float64) int {
	return int(math.Round(v))
}

// formatWeightKG matches the existing sheets' German-decimal-comma
// convention ("1,32 kg"), trimming a trailing zero/point.
func formatWeightKG(kg float64) string {
	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", kg), "0"), ".")
	return strings.Replace(s, ".", ",", 1) + " kg"
}
