package toolstar

import "testing"

// Text extracted from a real testLX summary PDF in GLPI
// (Testbericht-schnell-CZC8328H1S-HPProDesk600G3MT_001_P.pdf, 2026-09-18):
// labels and values land on separate lines.
const testReportPDFText = `
Testprotokoll: Schneller Hardwaretest
14 Tests in 4 Gruppen, 1x
Start: 2026-04-15 19:03:26
2026-04-15

PC-Name:
HP ProDesk 600 G3 MT
PC-Seriennr.:
CZC8328H1S
Firmenname:
AWO Akademie für Bildung und Integration gGmbH


Systemübersicht
CPU:
Intel(R) Core(TM) i5-7500 CPU @ 3.40GHz
Kerne/Threads:
1 CPU(s), 4 Kern(e) pro CPU, 1 Thread(s) pro Kern, 4 insg.
Speicher:
7928 MB
Module:
1x 8 GB - DDR4
Grafik:
Intel HD Graphics 630 - 1024 MB
Festplatte 1:
232,9 GB - Samsung SSD 850 EVO 250GB - SATA - SSD
Ergebnisse

Gesamtergebnis
bestanden
112x
0 Fehler
`

func TestParseTXT_TestReportPDFText(t *testing.T) {
	r := ParseTXT(testReportPDFText)
	checks := map[string][2]string{
		"PCName":       {r.PCName, "HP ProDesk 600 G3 MT"},
		"SerialNumber": {r.SerialNumber, "CZC8328H1S"},
		"CPUModel":     {r.CPUModel, "Intel(R) Core(TM) i5-7500 CPU @ 3.40GHz"},
		"GPUModel":     {r.GPUModel, "Intel HD Graphics 630"},
		"TestResult":   {r.TestResult, "bestanden"},
	}
	for field, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", field, c[0], c[1])
		}
	}
	if r.RAMTotalMB != 7928 {
		t.Errorf("RAMTotalMB = %d, want 7928", r.RAMTotalMB)
	}
	if !r.TestPassed {
		t.Error("TestPassed = false, want true")
	}
	if r.IsEraseCertificate() {
		t.Error("a test report must not look like an erase certificate")
	}
}

func TestFindLabelValue_EmptyValueBeforeNextLabel(t *testing.T) {
	if v := findLabelValue("PC-Name:\nPC-Seriennr.:\nX1", "PC-Name:"); v != "" {
		t.Errorf("got %q, want empty — the next line is another label", v)
	}
}
