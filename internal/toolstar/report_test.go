package toolstar

import "testing"

// Approximated plain text of a real shredderLX wipe certificate (PDF
// extraction runs adjacent table cells together with no reliable
// separator) — mirrors the sample used to validate the equivalent Rust
// parser in AWO-Software.
const sample = `Löschzertifikat 2026-05-28
PC-Name: ESPRIMO_P556
PC-Seriennr.: YM4X246512
Löschergebnis
Modell: SanDisk SD8SB8U128G-1006Seriennr.: 172986803982
Kapazität: 119,2 GB Sektoren: 250.069.680 Realloziert: 0
SMART-Status: okay SMARTBewertung:mittel Betriebszeit: 22.173
HPA: nicht vorhanden DCO: nicht vorhanden Accessible Max: nicht vorhanden
Löschmethode: NIST SP 800-88 Clear
Mit Überprüfung: ja, 10% Stichprobe
Dauer: 0:44:56
Ergebnis: erfolgreich
Systemübersicht
CPU: Intel(R) Pentium(R) CPU G4400 @ 3.30GHz
System: FUJITSU ESPRIMO_P556 S/N: YM4X246512
Unterschriften
Hiermit bestätige ich...
`

func TestParseTXT_EraseCertificate(t *testing.T) {
	r := ParseTXT(sample)

	if r.PCName != "ESPRIMO_P556" {
		t.Errorf("PCName = %q, want ESPRIMO_P556", r.PCName)
	}
	if r.SerialNumber != "YM4X246512" {
		t.Errorf("SerialNumber = %q, want YM4X246512", r.SerialNumber)
	}
	if !r.IsEraseCertificate() {
		t.Fatal("expected an erase certificate")
	}
	if len(r.EraseResults) != 1 {
		t.Fatalf("EraseResults len = %d, want 1", len(r.EraseResults))
	}

	e := r.EraseResults[0]
	if e.Serial != "172986803982" {
		t.Errorf("erase serial = %q, want 172986803982", e.Serial)
	}
	if e.CapacityGB != 119.2 {
		t.Errorf("erase capacity = %v, want 119.2", e.CapacityGB)
	}
	if e.EraseMethod != "NIST SP 800-88 Clear" {
		t.Errorf("erase method = %q, want %q", e.EraseMethod, "NIST SP 800-88 Clear")
	}
	if !e.Passed {
		t.Error("expected erase result to be marked passed")
	}
}

func TestParseTXT_MultipleDisks(t *testing.T) {
	text := "Löschergebnis\n" +
		"Modell: Disk A\n" +
		"Seriennr.: AAA111\n" +
		"Löschmethode: NIST SP 800-88 Purge\n" +
		"Ergebnis: erfolgreich\n" +
		"Modell: Disk B\n" +
		"Seriennr.: BBB222\n" +
		"Löschmethode: NIST SP 800-88 Clear\n" +
		"Ergebnis: fehlgeschlagen\n" +
		"Systemübersicht\n"

	r := ParseTXT(text)
	if len(r.EraseResults) != 2 {
		t.Fatalf("EraseResults len = %d, want 2", len(r.EraseResults))
	}
	if r.EraseResults[0].Serial != "AAA111" || !r.EraseResults[0].Passed {
		t.Errorf("disk 0 = %+v", r.EraseResults[0])
	}
	if r.EraseResults[1].Serial != "BBB222" || r.EraseResults[1].Passed {
		t.Errorf("disk 1 = %+v", r.EraseResults[1])
	}
}

func TestParseTXT_PlainTestReportHasNoEraseResults(t *testing.T) {
	text := "toolstar Start: 2026-05-28\nGesamtergebnis  bestanden\n"
	r := ParseTXT(text)
	if r.IsEraseCertificate() {
		t.Error("expected no erase results for a plain test report")
	}
	if !r.TestPassed || r.TestResult != "bestanden" {
		t.Errorf("TestResult = %q, TestPassed = %v", r.TestResult, r.TestPassed)
	}
}
