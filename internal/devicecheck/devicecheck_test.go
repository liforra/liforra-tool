package devicecheck

import (
	"testing"

	"liforra-tool/internal/glpi"
	"liforra-tool/internal/hwinfo"
	"liforra-tool/internal/toolstar"
	"liforra-tool/internal/usbscan"
)

func find(t *testing.T, results []Result, key string) *Result {
	t.Helper()
	for i := range results {
		if results[i].Key == key {
			return &results[i]
		}
	}
	return nil
}

func TestCheckDocuments_DiskMatchesEraseCertificate(t *testing.T) {
	info := hwinfo.Info{
		SerialNumber: "YM4X246512",
		Disks:        []hwinfo.Disk{{Model: "SanDisk SD8SB8U128G-1006"}},
	}
	files := []usbscan.FoundFile{
		{Kind: usbscan.KindEraseCertificate, Report: &toolstar.Report{
			EraseResults: []toolstar.EraseResult{{Model: "SanDisk SD8SB8U128G-1006", Passed: true}},
		}},
		{Kind: usbscan.KindTestReport, Report: &toolstar.Report{
			SerialNumber: "YM4X246512", TestPassed: true, TestResult: "bestanden",
		}},
	}

	results := CheckDocuments(info, files)
	if r := find(t, results, "erase.ok"); r == nil {
		t.Errorf("expected erase.ok, got %+v", results)
	}
	if r := find(t, results, "test.ok"); r == nil {
		t.Errorf("expected test.ok, got %+v", results)
	}
	for _, r := range results {
		if r.Status == StatusFail {
			t.Errorf("unexpected failure: %+v", r)
		}
	}
}

// The real-world failure mode this whole package exists for: someone
// attaches an erase certificate from a different drive/device to this one.
func TestCheckDocuments_WrongDiskInEraseCertificate(t *testing.T) {
	info := hwinfo.Info{
		Disks: []hwinfo.Disk{{Model: "Samsung SSD 850 EVO 250GB"}},
	}
	files := []usbscan.FoundFile{
		{Kind: usbscan.KindEraseCertificate, Report: &toolstar.Report{
			EraseResults: []toolstar.EraseResult{{Model: "WDC WD10JPVX-22JC3T0", Passed: true}},
		}},
	}

	results := CheckDocuments(info, files)
	r := find(t, results, "erase.diskMismatch")
	if r == nil {
		t.Fatalf("expected erase.diskMismatch, got %+v", results)
	}
	if r.Status != StatusFail {
		t.Errorf("status = %q, want fail", r.Status)
	}
	if r.Params["model"] != "Samsung SSD 850 EVO 250GB" {
		t.Errorf("params = %+v", r.Params)
	}
}

func TestCheckDocuments_FailedEraseIsFlagged(t *testing.T) {
	files := []usbscan.FoundFile{
		{Kind: usbscan.KindEraseCertificate, Report: &toolstar.Report{
			EraseResults: []toolstar.EraseResult{{Model: "Disk B", Result: "fehlgeschlagen", Passed: false}},
		}},
	}
	results := CheckDocuments(hwinfo.Info{}, files)
	r := find(t, results, "erase.failed")
	if r == nil || r.Status != StatusFail {
		t.Fatalf("expected erase.failed, got %+v", results)
	}
}

func TestCheckDocuments_TestReportSerialMismatch(t *testing.T) {
	info := hwinfo.Info{SerialNumber: "AAA111"}
	files := []usbscan.FoundFile{
		{Kind: usbscan.KindTestReport, Report: &toolstar.Report{SerialNumber: "BBB222", TestPassed: true, TestResult: "bestanden"}},
	}
	results := CheckDocuments(info, files)
	r := find(t, results, "test.serialMismatch")
	if r == nil || r.Status != StatusFail {
		t.Fatalf("expected test.serialMismatch, got %+v", results)
	}
}

func TestCheckDocuments_MissingDocumentsAreWarnings(t *testing.T) {
	results := CheckDocuments(hwinfo.Info{}, nil)
	if r := find(t, results, "erase.missing"); r == nil || r.Status != StatusWarn {
		t.Errorf("expected erase.missing warning, got %+v", results)
	}
	if r := find(t, results, "test.missing"); r == nil || r.Status != StatusWarn {
		t.Errorf("expected test.missing warning, got %+v", results)
	}
}

func TestCheckOS_CleanAccountNoUpdatesNoProblems(t *testing.T) {
	r := CheckOS(hwinfo.OSReadiness{LocalAccounts: []string{"User"}, PendingUpdates: 0})
	for _, key := range []string{"os.account.ok", "os.updates.ok", "os.devices.ok"} {
		if find(t, r, key) == nil {
			t.Errorf("expected %s, got %+v", key, r)
		}
	}
	for _, res := range r {
		if res.Status != StatusOK {
			t.Errorf("unexpected non-ok result: %+v", res)
		}
	}
}

func TestCheckOS_WrongAccountName(t *testing.T) {
	r := CheckOS(hwinfo.OSReadiness{LocalAccounts: []string{"John"}})
	res := find(t, r, "os.account.wrongName")
	if res == nil || res.Status != StatusWarn || res.Params["name"] != "John" {
		t.Fatalf("got %+v", r)
	}
}

func TestCheckOS_MultipleAccounts(t *testing.T) {
	r := CheckOS(hwinfo.OSReadiness{LocalAccounts: []string{"User", "John"}})
	res := find(t, r, "os.account.multiple")
	if res == nil || res.Status != StatusWarn || res.Params["count"] != "2" {
		t.Fatalf("got %+v", r)
	}
}

func TestCheckOS_PendingUpdates(t *testing.T) {
	r := CheckOS(hwinfo.OSReadiness{PendingUpdates: 3})
	res := find(t, r, "os.updates.pending")
	if res == nil || res.Status != StatusWarn || res.Params["count"] != "3" {
		t.Fatalf("got %+v", r)
	}
}

func TestCheckOS_UnknownUpdateStatusIsNotTreatedAsOK(t *testing.T) {
	r := CheckOS(hwinfo.OSReadiness{PendingUpdates: -1})
	res := find(t, r, "os.updates.unknown")
	if res == nil || res.Status != StatusWarn {
		t.Fatalf("got %+v", r)
	}
}

func TestCheckOS_ProblemDevicesAreFailures(t *testing.T) {
	r := CheckOS(hwinfo.OSReadiness{ProblemDevices: []string{"Unknown device (Code 28)"}})
	res := find(t, r, "os.devices.problem")
	if res == nil || res.Status != StatusFail || res.Params["device"] != "Unknown device (Code 28)" {
		t.Fatalf("got %+v", r)
	}
}

func TestCheckGLPI_DuplicateSerial(t *testing.T) {
	matches := []glpi.Computer{{ID: 1, Name: "#3303"}, {ID: 2, Name: "#3303-dup"}}
	results := CheckGLPI(hwinfo.Info{}, matches)
	if len(results) != 1 || results[0].Key != "glpi.duplicate" || results[0].Status != StatusFail {
		t.Fatalf("got %+v", results)
	}
	if results[0].Params["count"] != "2" {
		t.Errorf("params = %+v", results[0].Params)
	}
}

func TestCheckGLPI_NotFoundIsAWarningNotAFailure(t *testing.T) {
	results := CheckGLPI(hwinfo.Info{}, nil)
	if len(results) != 1 || results[0].Key != "glpi.notFound" || results[0].Status != StatusWarn {
		t.Fatalf("got %+v", results)
	}
}

func TestCheckGLPI_ManufacturerMismatch(t *testing.T) {
	info := hwinfo.Info{Manufacturer: "LENOVO", Model: "ThinkPad T470s", DeviceType: "Laptop"}
	matches := []glpi.Computer{{
		Manufacturer: glpi.NamedRef{Name: "Dell"},
		Model:        glpi.NamedRef{Name: "Latitude 5480"},
		Type:         glpi.NamedRef{Name: "Laptop"},
	}}
	results := CheckGLPI(info, matches)
	r := find(t, results, "glpi.mismatchManufacturer")
	if r == nil || r.Status != StatusWarn {
		t.Fatalf("expected glpi.mismatchManufacturer, got %+v", results)
	}
	if r.Params["glpi"] != "Dell" || r.Params["scan"] != "LENOVO" {
		t.Errorf("params = %+v", r.Params)
	}
}

func TestCheckGLPI_ModelWithBrandPrefixStillMatches(t *testing.T) {
	// GLPI's own model dropdown usually includes the brand ("Lenovo
	// ThinkPad T470s"); the live scan's is bare ("ThinkPad T470s") — that
	// alone must not be flagged as a mismatch.
	info := hwinfo.Info{Manufacturer: "LENOVO", Model: "ThinkPad T470s", DeviceType: "Laptop"}
	matches := []glpi.Computer{{
		Manufacturer: glpi.NamedRef{Name: "Lenovo"},
		Model:        glpi.NamedRef{Name: "Lenovo ThinkPad T470s"},
		Type:         glpi.NamedRef{Name: "Laptop"},
	}}
	results := CheckGLPI(info, matches)
	if r := find(t, results, "glpi.ok"); r == nil {
		t.Errorf("expected glpi.ok, got %+v", results)
	}
	for _, r := range results {
		if r.Status != StatusOK {
			t.Errorf("unexpected non-ok result: %+v", r)
		}
	}
}
