// Package devicecheck cross-checks a device against itself: does the erase
// certificate on the USB stick actually name the disk installed in this
// machine, does the test report's serial match what's actually plugged in
// right now, is there more than one GLPI record for this serial, does the
// GLPI record's manufacturer/model/type roughly agree with what's
// physically detected. Nothing here is a GLPI write — read-only sanity
// checks for a technician to look at before finishing a device.
//
// Every result carries a Key and Params rather than a baked-in message —
// the frontend supplies the localized label/detail text (see i18n.ts),
// same as the rest of the app.
package devicecheck

import (
	"regexp"
	"strconv"
	"strings"

	"liforra-tool/internal/glpi"
	"liforra-tool/internal/hwinfo"
	"liforra-tool/internal/usbscan"
)

type Status string

const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

type Result struct {
	Key    string            `json:"key"`
	Status Status            `json:"status"`
	Params map[string]string `json:"params,omitempty"`
}

func ok(key string, params ...string) Result   { return build(StatusOK, key, params) }
func warn(key string, params ...string) Result { return build(StatusWarn, key, params) }
func fail(key string, params ...string) Result { return build(StatusFail, key, params) }

// build takes params as alternating key/value pairs — build("x", "a", "1", "b", "2").
func build(status Status, key string, params []string) Result {
	r := Result{Key: key, Status: status}
	if len(params) > 0 {
		r.Params = map[string]string{}
		for i := 0; i+1 < len(params); i += 2 {
			r.Params[params[i]] = params[i+1]
		}
	}
	return r
}

var nonAlnumRe = regexp.MustCompile(`[^a-z0-9]+`)

func tokens(s string) map[string]bool {
	m := map[string]bool{}
	norm := nonAlnumRe.ReplaceAllString(strings.ToLower(s), " ")
	for _, t := range strings.Fields(norm) {
		m[t] = true
	}
	return m
}

// subsetMatch reports whether every meaningful word of want appears in
// have — the same "every word must appear" idea the GLPI catalog matcher
// uses (internal/glpi/match.go), just without its noise-word/alias
// handling, since these strings (disk model firmware names, GLPI's own
// manufacturer/model dropdowns) are shorter and more consistently spelled.
func subsetMatch(have, want string) bool {
	if have == "" || want == "" {
		return false
	}
	h := tokens(have)
	w := tokens(want)
	if len(w) == 0 {
		return false
	}
	for t := range w {
		if !h[t] {
			return false
		}
	}
	return true
}

// findTestReport/findEraseCertificate return the first of each kind — a
// USB stick normally carries at most one of each.
func findTestReport(files []usbscan.FoundFile) *usbscan.FoundFile {
	for i := range files {
		if files[i].Kind == usbscan.KindTestReport {
			return &files[i]
		}
	}
	return nil
}

func findEraseCertificate(files []usbscan.FoundFile) *usbscan.FoundFile {
	for i := range files {
		if files[i].Kind == usbscan.KindEraseCertificate {
			return &files[i]
		}
	}
	return nil
}

// CheckDocuments cross-checks the live hardware scan against whatever
// erase certificate / test report were found on the USB stick. Pure — no
// GLPI involved, safe to call before a device even exists there.
func CheckDocuments(info hwinfo.Info, files []usbscan.FoundFile) []Result {
	var results []Result

	if erase := findEraseCertificate(files); erase == nil {
		results = append(results, warn("erase.missing"))
	} else if erase.Report != nil {
		for _, e := range erase.Report.EraseResults {
			if !e.Passed {
				results = append(results, fail("erase.failed", "model", e.Model, "result", e.Result))
			}
		}

		installed := 0
		for _, d := range info.Disks {
			if d.Model == "" {
				continue // can't check what wasn't detected
			}
			installed++
			matched := false
			for _, e := range erase.Report.EraseResults {
				if subsetMatch(d.Model, e.Model) || subsetMatch(e.Model, d.Model) {
					matched = true
					break
				}
			}
			if !matched {
				results = append(results, fail("erase.diskMismatch", "model", d.Model))
			}
		}
		if installed > 0 && len(erase.Report.EraseResults) != installed {
			results = append(results, warn("erase.countMismatch",
				"erased", strconv.Itoa(len(erase.Report.EraseResults)), "installed", strconv.Itoa(installed)))
		}
		if len(results) == 0 {
			results = append(results, ok("erase.ok"))
		}
	}

	if test := findTestReport(files); test == nil {
		results = append(results, warn("test.missing"))
	} else if test.Report != nil {
		r := test.Report
		switch {
		case !r.TestPassed && r.TestResult != "":
			results = append(results, fail("test.failed", "result", r.TestResult))
		case r.SerialNumber != "" && info.SerialNumber != "" && !strings.EqualFold(r.SerialNumber, info.SerialNumber):
			results = append(results, fail("test.serialMismatch", "reportSerial", r.SerialNumber, "scanSerial", info.SerialNumber))
		default:
			results = append(results, ok("test.ok"))
		}
	}

	return results
}

// CheckOS reports on OS-level "ready to hand off" signals — pure, given
// already-gathered data (see hwinfo.DetectOSReadiness, which is separate
// from the hardware scan because a Windows Update search can be slow).
func CheckOS(r hwinfo.OSReadiness) []Result {
	var results []Result

	switch len(r.LocalAccounts) {
	case 0:
		results = append(results, warn("os.account.none"))
	case 1:
		if r.LocalAccounts[0] == "User" {
			results = append(results, ok("os.account.ok"))
		} else {
			results = append(results, warn("os.account.wrongName", "name", r.LocalAccounts[0]))
		}
	default:
		results = append(results, warn("os.account.multiple",
			"count", strconv.Itoa(len(r.LocalAccounts)), "names", strings.Join(r.LocalAccounts, ", ")))
	}

	switch {
	case r.PendingUpdates < 0:
		results = append(results, warn("os.updates.unknown"))
	case r.PendingUpdates == 0:
		results = append(results, ok("os.updates.ok"))
	default:
		results = append(results, warn("os.updates.pending", "count", strconv.Itoa(r.PendingUpdates)))
	}

	if len(r.ProblemDevices) == 0 {
		results = append(results, ok("os.devices.ok"))
	}
	for _, d := range r.ProblemDevices {
		results = append(results, fail("os.devices.problem", "device", d))
	}

	return results
}

// CheckGLPI cross-checks how many existing Computer records share this
// serial, and — for a single match — whether its manufacturer/model/type
// roughly agree with what's physically detected. Call with whatever
// glpi.FindBySerial(info.SerialNumber) returned.
func CheckGLPI(info hwinfo.Info, matches []glpi.Computer) []Result {
	switch len(matches) {
	case 0:
		return []Result{warn("glpi.notFound")}
	case 1:
		// fall through to the field comparison below
	default:
		return []Result{fail("glpi.duplicate", "count", strconv.Itoa(len(matches)))}
	}

	c := matches[0]
	var results []Result
	if name := c.Manufacturer.Name; name != "" && info.Manufacturer != "" && !strings.EqualFold(name, info.Manufacturer) {
		results = append(results, warn("glpi.mismatchManufacturer", "glpi", name, "scan", info.Manufacturer))
	}
	if name := c.Model.Name; name != "" && info.Model != "" && !subsetMatch(name, info.Model) {
		results = append(results, warn("glpi.mismatchModel", "glpi", name, "scan", info.Model))
	}
	if name := c.Type.Name; name != "" && info.DeviceType != "" && !strings.EqualFold(name, info.DeviceType) {
		results = append(results, warn("glpi.mismatchType", "glpi", name, "scan", info.DeviceType))
	}
	if len(results) == 0 {
		results = append(results, ok("glpi.ok"))
	}
	return results
}
