// Package lenovospecs is a best-effort lookup of weight, dimensions and
// ports from Lenovo's own published spec sheet (PSREF — Product
// Specifications Reference) for a given model, e.g. "ThinkPad T470s".
//
// Lenovo's PSREF PDFs happen to live at a predictable URL
// (https://psref.lenovo.com/syspool/Sys/PDF/<Family>/<Family_Rest>/<Family_Rest>_Spec.PDF,
// confirmed 2026-09 against a real ThinkPad T470s sheet), unlike Dell/HP/
// Fujitsu/Apple, none of which have an equivalent guessable source (checked
// the same day — no luck). That URL scheme is not documented by Lenovo, so
// it's a guess that can silently stop working for any given model, or if
// Lenovo restructures the site: Lookup returns nil rather than an error in
// every such case, exactly like a prefill field that just came back empty.
// Nothing here is ever authoritative; every value it finds is a prefill the
// technician can still overwrite.
package lenovospecs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

type Ports struct {
	VGA         int `json:"vga"`
	DisplayPort int `json:"displayPort"`
	HDMI        int `json:"hdmi"`
	DVI         int `json:"dvi"`
	USB3        int `json:"usb3"`
	USB2        int `json:"usb2"`
	Ethernet    int `json:"ethernet"`
	Kopfhoerer  int `json:"kopfhoerer"`
	Mikrofon    int `json:"mikrofon"`
	LineIn      int `json:"lineIn"`
	PS2         int `json:"ps2"`
}

type Result struct {
	WeightKG float64 `json:"weightKG"`
	// WidthMM/HeightMM/DepthMM are 0 if not found. HeightMM is the upper
	// bound when the sheet gives a range (e.g. "16.9-18.8mm").
	WidthMM  float64 `json:"widthMM"`
	HeightMM float64 `json:"heightMM"`
	DepthMM  float64 `json:"depthMM"`
	Ports    Ports   `json:"ports"`
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// specURL guesses the PSREF PDF path from a resolved model name.
func specURL(modelName string) string {
	slug := strings.Join(strings.Fields(modelName), "_")
	if slug == "" {
		return ""
	}
	family := slug
	if i := strings.Index(slug, "_"); i != -1 {
		family = slug[:i]
	}
	return fmt.Sprintf("https://psref.lenovo.com/syspool/Sys/PDF/%s/%s/%s_Spec.PDF", family, slug, slug)
}

// Lookup tries to fetch and parse Lenovo's spec sheet for modelName. Returns
// nil whenever anything doesn't pan out — wrong URL guess, network error,
// non-200, or a PDF layout the parser doesn't recognize. Never an error the
// caller needs to handle differently from "no data available".
func Lookup(ctx context.Context, modelName string) *Result {
	url := specURL(modelName)
	if url == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	// PSREF sheets are a few hundred KB; bail out rather than trust an
	// arbitrarily large response.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil
	}

	text, err := extractText(body)
	if err != nil || text == "" {
		return nil
	}
	r := parse(text)
	if r == (Result{}) {
		return nil
	}
	return &r
}

func extractText(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		t, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(t)
		buf.WriteString("\n")
	}
	return buf.String(), nil
}

var (
	// "WxDxH: 13.03" x 8.93" x 0.67-0.74"; 331mm x 226.8mm x 16.9-18.8mm" —
	// only the mm figures are used, the inch ones are skipped over.
	dimensionsRe = regexp.MustCompile(`WxDxH:.*?([\d.]+)mm\s*x\s*([\d.]+)mm\s*x\s*([\d.]+)(?:-([\d.]+))?mm`)
	// "Starting at 2.9 lb / 1.32 kg" — the common laptop phrasing. Sheets
	// with a configurable weight instead print a table ("Magnesium 3.49 lb
	// / 1.58 kg 3.92 lb / 1.78 kg ..."), and PDF extraction runs adjacent
	// table cells together with no separator at all — confirmed live on a
	// real ThinkPad T470 sheet, whose "kg" is followed immediately by the
	// next cell's digit, not even a space. So weightBareRe deliberately has
	// no trailing \b (a word boundary needs a non-word character on one
	// side; two adjacent word characters like "g" and "3" never form one)
	// and just takes the first, lightest configuration.
	weightStartingAtRe = regexp.MustCompile(`Starting at[^/\n]*/\s*([\d.]+)\s*kg`)
	weightBareRe       = regexp.MustCompile(`([\d.]+)\s*kg`)
	ethernetRe         = regexp.MustCompile(`Ethernet\s*\(RJ-45\)`)
	comboAudioJackRe   = regexp.MustCompile(`combo audio.?(?:/|and )?microphone jack`)
	countWordRe        = regexp.MustCompile(`\b(One|Two|Three|Four|Five|Six)\s+USB`)
	usb2CountRe        = regexp.MustCompile(`\b(One|Two|Three|Four|Five|Six)\s+USB\s*2\.0`)
	hdmiCountRe        = regexp.MustCompile(`\b(One|Two|Three|Four|Five|Six)\s+HDMI`)
)

var numberWords = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6}

func wordToN(s string) int {
	if n, ok := numberWords[strings.ToLower(s)]; ok {
		return n
	}
	return 1
}

func parseF(s string) float64 {
	n, _ := strconv.ParseFloat(s, 64)
	return n
}

// parse extracts what it can from a PSREF sheet's full text. Dimensions and
// weight are found by distinctive, sheet-wide phrases Lenovo uses
// consistently. Ports are read from a bounded window ending at the
// "Ethernet (RJ-45)" anchor (every laptop/desktop sheet has exactly one),
// rather than searching the whole document — PSREF sheets also mention
// "HDMI to VGA" / "USB-C to VGA" a little further down, describing optional
// adapters, not built-in ports, and those must not be counted.
func parse(text string) Result {
	var r Result

	if m := dimensionsRe.FindStringSubmatch(text); m != nil {
		r.WidthMM = parseF(m[1])
		r.DepthMM = parseF(m[2])
		if m[4] != "" {
			r.HeightMM = parseF(m[4])
		} else {
			r.HeightMM = parseF(m[3])
		}
	}

	if m := weightStartingAtRe.FindStringSubmatch(text); m != nil {
		r.WeightKG = parseF(m[1])
	} else if m := weightBareRe.FindStringSubmatch(text); m != nil {
		r.WeightKG = parseF(m[1])
	}

	if loc := ethernetRe.FindStringIndex(text); loc != nil {
		start := loc[1] - 400
		if start < 0 {
			start = 0
		}
		window := text[start:loc[1]]
		r.Ports.Ethernet = 1

		if m := countWordRe.FindStringSubmatch(window); m != nil {
			r.Ports.USB3 = wordToN(m[1])
		} else if strings.Contains(window, "USB 3") {
			r.Ports.USB3 = 1
		}
		if m := usb2CountRe.FindStringSubmatch(window); m != nil {
			r.Ports.USB2 = wordToN(m[1])
			// A sheet listing both USB 2.0 and USB 3.x separately still
			// only has one "One/Two/... USB" count word covered above, so
			// this doesn't double-count — but if the only match was the
			// generic USB3 pattern picking up the 2.0 phrase by mistake,
			// prefer the more specific one.
			if r.Ports.USB3 == r.Ports.USB2 && !strings.Contains(window, "USB 3") {
				r.Ports.USB3 = 0
			}
		}
		if strings.Contains(window, "Type-C") || strings.Contains(window, "Thunderbolt") {
			// USB-C/Thunderbolt has no dedicated field in this shop's
			// spec-sheet format — folded into USB3, same as a regular
			// USB 3.x port.
			r.Ports.USB3++
		}
		if m := hdmiCountRe.FindStringSubmatch(window); m != nil {
			r.Ports.HDMI = wordToN(m[1])
		} else if strings.Contains(window, "HDMI") {
			r.Ports.HDMI = 1
		}
		if strings.Contains(window, "DisplayPort") {
			r.Ports.DisplayPort = 1
		}
		// "VGA"/"DVI" as a real port, not "... to VGA:" adapter wording —
		// safe here because the window ends at "Ethernet (RJ-45)", before
		// any such adapter line appears in a real PSREF sheet.
		if regexp.MustCompile(`\bVGA\b`).MatchString(window) {
			r.Ports.VGA = 1
		}
		if regexp.MustCompile(`\bDVI\b`).MatchString(window) {
			r.Ports.DVI = 1
		}
	}

	if comboAudioJackRe.MatchString(text) {
		r.Ports.Kopfhoerer = 1
		r.Ports.Mikrofon = 1
	}

	return r
}
