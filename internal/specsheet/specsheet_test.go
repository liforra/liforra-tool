package specsheet

import (
	"strings"
	"testing"
)

func TestGenerate_MatchesKnownFormat(t *testing.T) {
	out := Generate(Fields{
		SKU:        "A12345",
		Geraetetyp: "Laptop",
		Marke:      "Dell",
		Modell:     "Latitude 7390",
		Prozessor:  "Intel Core i5-8350U",
		RAM:        "8.0 GB DDR3",
		WLAN:       true,
		Bluetooth:  false,
		Ports:      Ports{HDMI: 1, USB3: 2},
		Anwendungsgebiete: Anwendungsgebiete{
			Bueroarbeit: true,
			Gaming:      false,
		},
		Lieferumfang: Lieferumfang{
			PCSystem: true,
			Kabel:    true,
		},
		Bemerkungen: "Kratzer am Deckel",
	})

	checks := []string{
		"SKU (Artikelnummer): A12345",
		"Gerätetyp: Laptop",
		"PC-System",
		"Systemkonfiguration",
		"» Marke: Dell",
		"» Modell: Latitude 7390",
		"» Prozessor: Intel Core i5-8350U",
		"» Arbeitsspeicher (RAM): 8.0 GB DDR3",
		"Konnektivität",
		"» WLAN: Vorhanden",
		"» Bluetooth: Nicht vorhanden",
		"» SD-Kartensteckplatz: nicht vorhanden",
		"Anschlüsse",
		"» 1x HDMI",
		"» 2x USB 3.0/3.1 Gen 1 (Typ-A)",
		"Anwendungsgebiete",
		"» Büroarbeit: ✓",
		"» Gaming: ✗",
		"Lieferumfang",
		"» PC-System",
		"» Stromkabel für das PC-System",
		"Bemerkungen",
		"Kratzer am Deckel",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing expected line %q\n--- full output ---\n%s", want, out)
		}
	}

	// Like the sheets already in GLPI: empty fields keep their label, ports
	// that aren't there aren't listed, CRLF with a trailing line break.
	for _, want := range []string{"» Mainboard: \r\n", "» Farbe: \r\n", "» Laufwerk: \r\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing empty field line %q\n--- full output ---\n%s", want, out)
		}
	}
	if strings.Contains(out, "DVI") {
		t.Errorf("output lists a port with count 0\n--- full output ---\n%s", out)
	}
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") || !strings.HasSuffix(out, "\r\n") {
		t.Errorf("output must use CRLF line endings and end with one")
	}
}

func TestGenerate_EmptySectionsKeepHeaders(t *testing.T) {
	out := Generate(Fields{})
	for _, want := range []string{"SKU (Artikelnummer): \r\n", "Gerätetyp: \r\n", "\r\nAnschlüsse\r\n\r\nGewicht und Größe\r\n", "\r\nLieferumfang\r\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- full output ---\n%s", want, out)
		}
	}
	if strings.Contains(out, "Bemerkungen") {
		t.Errorf("empty Bemerkungen must not produce a section")
	}
}
