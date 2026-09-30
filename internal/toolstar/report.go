// Package toolstar parses the TXT reports produced by toolstar®shredderLX
// (disk wipe/erase certificates) and toolstar®testLX (hardware diagnostic
// reports) — the "test and delete documents" a technician finds on a USB
// stick after running those tools on a device.
//
// Format details (German-language reports, confirmed against a real sample
// pulled from GLPI, and against the erase-certificate parsing logic in
// AWO-Software's toolstar.rs — see project memory) are not exhaustively
// documented anywhere, so this parser only covers the fields this tool
// actually needs. Extend it if a real-world report doesn't parse cleanly.
package toolstar

import (
	"regexp"
	"strconv"
	"strings"
)

type RAMModule struct {
	Index        int    `json:"index"`
	SizeMB       int    `json:"sizeMB"`
	DDRType      string `json:"ddrType"`
	SpeedMHz     int    `json:"speedMHz"`
	Manufacturer string `json:"manufacturer"`
	PartNumber   string `json:"partNumber"`
}

type Disk struct {
	Model      string  `json:"model"`
	Serial     string  `json:"serial"`
	CapacityGB float64 `json:"capacityGB"`
	Interface  string  `json:"interface"`
	IsSSD      bool    `json:"isSSD"`
}

// EraseResult is one physical drive's entry in a shredderLX wipe certificate.
type EraseResult struct {
	Model       string  `json:"model"`
	Serial      string  `json:"serial"`
	CapacityGB  float64 `json:"capacityGB"`
	EraseMethod string  `json:"eraseMethod"`
	Result      string  `json:"result"`
	Passed      bool    `json:"passed"`
}

type Report struct {
	PCName       string      `json:"pcName"`
	SerialNumber string      `json:"serialNumber"`
	CPUModel     string      `json:"cpuModel"`
	RAMTotalMB   int         `json:"ramTotalMB"`
	RAMModules   []RAMModule `json:"ramModules"`
	Disks        []Disk      `json:"disks"`
	GPUModel     string      `json:"gpuModel"`
	NetworkCard  string      `json:"networkCard"`
	NetworkMAC   string      `json:"networkMAC"`
	TestDate     string      `json:"testDate"`
	TestResult   string      `json:"testResult"`
	TestPassed   bool        `json:"testPassed"`

	// EraseResults is non-empty only for a shredderLX wipe certificate.
	EraseResults []EraseResult `json:"eraseResults"`
}

// IsEraseCertificate reports whether this looks like a shredderLX wipe
// certificate rather than (or in addition to) a testLX test report.
func (r *Report) IsEraseCertificate() bool {
	return len(r.EraseResults) > 0
}

var deFloatRe = regexp.MustCompile(`^[\d.,]+`)

func parseDEFloat(s string) float64 {
	m := deFloatRe.FindString(strings.TrimSpace(s))
	if m == "" {
		return 0
	}
	// German formatting: '.' thousands separator, ',' decimal separator.
	m = strings.ReplaceAll(m, ".", "")
	m = strings.ReplaceAll(m, ",", ".")
	f, _ := strconv.ParseFloat(m, 64)
	return f
}

func parseDEInt(s string) int {
	return int(parseDEFloat(s))
}

// eraseLabels are the known field labels inside a shredderLX result block.
// PDF/TXT extraction sometimes runs adjacent cells together with no
// separator (e.g. "Modell: X Seriennr.: Y"), so we hunt for labels in the
// flowing text rather than parsing line by line.
// Includes labels we don't currently capture a value for (Sektoren, SMART
// status, HPA/DCO, ...) purely so they act as boundary markers — without
// them, the text between two *tracked* labels would swallow whatever
// untracked fields sit in between.
var eraseLabels = []string{
	"Modell:", "Seriennr.:", "Kapazität:", "Sektoren:", "Realloziert:",
	"SMART-Status:", "SMART-Bewertung:", "SMARTBewertung:", "Betriebszeit:",
	"HPA:", "DCO:", "Accessible Max:", "Löschmethode:", "Mit Überprüfung:",
	"Dauer:", "Ergebnis:",
}

func eraseFieldKey(label string) string {
	switch label {
	case "Seriennr.:":
		return "Seriennr."
	case "Kapazität:":
		return "Kapazität"
	case "Löschmethode:":
		return "Löschmethode"
	case "Ergebnis:":
		return "Ergebnis"
	}
	return ""
}

func applyEraseField(cur *EraseResult, key, val string) {
	switch key {
	case "Seriennr.":
		cur.Serial = val
	case "Kapazität":
		cur.CapacityGB = parseDEFloat(val)
	case "Löschmethode":
		cur.EraseMethod = val
	case "Ergebnis":
		cur.Result = val
		cur.Passed = strings.EqualFold(strings.TrimSpace(val), "erfolgreich")
	}
}

// parseEraseResults finds the block between "Löschergebnis" and
// "Systemübersicht"/"Unterschriften" and splits it into one EraseResult per
// physical drive.
func parseEraseResults(text string) []EraseResult {
	var results []EraseResult

	start := strings.Index(text, "Löschergebnis")
	if start == -1 {
		return results
	}
	rest := text[start+len("Löschergebnis"):]

	end := len(rest)
	if i := strings.Index(rest, "Systemübersicht"); i != -1 && i < end {
		end = i
	}
	if i := strings.Index(rest, "Unterschriften"); i != -1 && i < end {
		end = i
	}
	block := rest[:end]

	var current *EraseResult
	pos := 0
	var pendingKey string
	pendingStart := -1

	for {
		nextIdx, nextLabel := -1, ""
		for _, label := range eraseLabels {
			if i := strings.Index(block[pos:], label); i != -1 {
				abs := pos + i
				if nextIdx == -1 || abs < nextIdx {
					nextIdx, nextLabel = abs, label
				}
			}
		}

		if pendingStart != -1 {
			valEnd := len(block)
			if nextIdx != -1 {
				valEnd = nextIdx
			}
			rawVal := strings.TrimSpace(block[pendingStart:valEnd])
			if pendingKey == "Modell" {
				if current != nil {
					results = append(results, *current)
				}
				current = &EraseResult{Model: rawVal}
			} else if current != nil {
				applyEraseField(current, pendingKey, rawVal)
			}
		}

		if nextIdx == -1 {
			break
		}
		key := eraseFieldKey(nextLabel)
		if nextLabel == "Modell:" {
			key = "Modell"
		}
		pendingKey = key
		pendingStart = nextIdx + len(nextLabel)
		pos = pendingStart
	}

	if current != nil {
		results = append(results, *current)
	}
	return results
}

func kv(line string) (string, string, bool) {
	i := strings.Index(line, ":")
	if i == -1 {
		return "", "", false
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
}

// findLabelValue returns the value after label: on the same line (TXT
// reports), or on the next non-empty line — text extracted from toolstar's
// PDFs puts label and value in separate cells ("PC-Name:\nHP ProDesk ...").
// A next line that is itself a label ("...:") means the value is empty.
func findLabelValue(text, label string) string {
	idx := strings.Index(text, label)
	if idx == -1 {
		return ""
	}
	return valueAfter(text[idx+len(label):])
}

// findLineLabelValue is findLabelValue for a label that must start its line,
// for short labels ("CPU:") that also occur inside longer ones.
func findLineLabelValue(text, label string) string {
	loc := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(label)).FindStringIndex(text)
	if loc == nil {
		return ""
	}
	return valueAfter(text[loc[1]:])
}

func valueAfter(after string) string {
	lines := strings.Split(after, "\n")
	if v := strings.TrimSpace(lines[0]); v != "" {
		return v
	}
	for _, l := range lines[1:] {
		if v := strings.TrimSpace(l); v != "" {
			if strings.HasSuffix(v, ":") {
				return ""
			}
			return v
		}
	}
	return ""
}

// ParseTXT parses a toolstar TXT report (shredderLX wipe certificate,
// testLX test report, or both).
func ParseTXT(content string) *Report {
	r := &Report{}

	if v := findLabelValue(content, "PC-Name:"); v != "" {
		r.PCName = v
	}
	if v := findLabelValue(content, "PC-Seriennr.:"); v != "" {
		r.SerialNumber = v
	}
	r.EraseResults = parseEraseResults(content)

	inMemoryTest := false
	inNetworkTest := false
	inGPUTest := false

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// PDF summary: "Gesamtergebnis" alone, the result on the next line.
		if trimmed == "Gesamtergebnis" {
			if v := valueAfter(strings.Join(lines[i+1:], "\n")); v != "" {
				r.TestResult = v
				r.TestPassed = v == "bestanden"
			}
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "Test 1.1:"):
			inMemoryTest = !strings.Contains(trimmed, "Ende")
			continue
		case strings.HasPrefix(trimmed, "Test 2.2:"):
			inNetworkTest = !strings.Contains(trimmed, "Ende")
			continue
		case strings.HasPrefix(trimmed, "Test 2.5:"):
			inGPUTest = !strings.Contains(trimmed, "Ende")
			continue
		}

		if strings.HasPrefix(trimmed, "toolstar") && strings.Contains(trimmed, "Start:") {
			after := trimmed[strings.Index(trimmed, "Start:")+len("Start:"):]
			fields := strings.Fields(after)
			if len(fields) > 0 {
				r.TestDate = fields[0]
			}
			continue
		}

		// "Gesamtergebnis  bestanden" (no colon) is the overall pass/fail line.
		if strings.HasPrefix(trimmed, "Gesamtergebnis") && !strings.Contains(trimmed, ":") {
			fields := strings.Fields(trimmed)
			if len(fields) > 1 {
				r.TestResult = fields[1]
				r.TestPassed = fields[1] == "bestanden"
			}
			continue
		}

		key, val, ok := kv(trimmed)
		if !ok {
			continue
		}

		switch {
		case key == "Speichergröße" && inMemoryTest:
			r.RAMTotalMB = parseDEInt(val)
		case key == "Device" && inGPUTest:
			r.GPUModel = val
		case key == "Netzwerkkarte" && inNetworkTest:
			if i := strings.Index(val, ":"); i != -1 {
				val = strings.TrimSpace(val[i+1:])
			}
			if r.NetworkCard == "" {
				r.NetworkCard = val
			}
		case key == "MAC-Adresse" && inNetworkTest:
			if r.NetworkMAC == "" {
				r.NetworkMAC = val
			}
		}
	}

	// The "Systemübersicht" summary at the top of a testLX report (in the
	// PDF version the only place these appear).
	if r.CPUModel == "" {
		r.CPUModel = findLineLabelValue(content, "CPU:")
	}
	if r.RAMTotalMB == 0 {
		r.RAMTotalMB = parseDEInt(findLineLabelValue(content, "Speicher:"))
	}
	if r.GPUModel == "" {
		// "Intel HD Graphics 630 - 1024 MB"
		gpu := findLineLabelValue(content, "Grafik:")
		if i := strings.Index(gpu, " - "); i != -1 {
			gpu = gpu[:i]
		}
		r.GPUModel = gpu
	}

	return r
}
