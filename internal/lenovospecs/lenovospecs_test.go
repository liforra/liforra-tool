package lenovospecs

import (
	"os"
	"testing"
)

func TestSpecURL(t *testing.T) {
	got := specURL("ThinkPad T470s")
	want := "https://psref.lenovo.com/syspool/Sys/PDF/ThinkPad/ThinkPad_T470s/ThinkPad_T470s_Spec.PDF"
	if got != want {
		t.Errorf("specURL = %q, want %q", got, want)
	}
}

// Real Lenovo PSREF sheet, fetched 2026-09-29 from the URL specURL guesses
// for this exact model — confirms the parser against real vendor data, not
// hand-written fixture text.
func TestParse_RealThinkPadT470sSheet(t *testing.T) {
	data, err := os.ReadFile("testdata/ThinkPad_T470s_Spec.pdf")
	if err != nil {
		t.Fatal(err)
	}
	text, err := extractText(data)
	if err != nil {
		t.Fatal(err)
	}
	r := parse(text)

	want := Result{
		WeightKG: 1.32,
		WidthMM:  331,
		DepthMM:  226.8,
		HeightMM: 18.8,
		Ports: Ports{
			Ethernet:   1,
			USB3:       4, // three USB-A 3.1 Gen 1 + one USB-C/Thunderbolt
			HDMI:       1,
			Kopfhoerer: 1,
			Mikrofon:   1,
		},
	}
	if r != want {
		t.Errorf("parse() = %+v, want %+v", r, want)
	}
}

// A second real sheet, deliberately different in shape: its weight is a
// configurable table ("Magnesium 3.49 lb / 1.58 kg 3.92 lb / 1.78 kg ..."),
// not the single "Starting at" line the T470s uses — PDF extraction glues
// that table's cells together with no separator, which is exactly the case
// that broke weightBareRe's trailing \b (see lenovospecs.go's comment).
func TestParse_RealThinkPadT470Sheet(t *testing.T) {
	data, err := os.ReadFile("testdata/ThinkPad_T470_Spec.pdf")
	if err != nil {
		t.Fatal(err)
	}
	text, err := extractText(data)
	if err != nil {
		t.Fatal(err)
	}
	r := parse(text)

	want := Result{
		WeightKG: 1.58, // first (lightest: Magnesium, 3-cell) row of the table
		WidthMM:  336.6,
		DepthMM:  232.5,
		HeightMM: 19.95,
		Ports: Ports{
			Ethernet:   1,
			USB3:       4,
			HDMI:       1,
			Kopfhoerer: 1,
			Mikrofon:   1,
		},
	}
	if r != want {
		t.Errorf("parse() = %+v, want %+v", r, want)
	}
}

func TestParse_UnrelatedTextReturnsZeroValue(t *testing.T) {
	if r := parse("nothing useful here"); r != (Result{}) {
		t.Errorf("parse() = %+v, want zero value", r)
	}
}
