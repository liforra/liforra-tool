// Package usbscan finds removable drives and classifies the toolstar
// reports (and any finished-device TXT spec sheet) sitting on them.
package usbscan

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
	"liforra-tool/internal/toolstar"
)

type Drive struct {
	// Path is the filesystem root to scan (e.g. "E:\" on Windows).
	Path  string `json:"path"`
	Label string `json:"label"`
}

// FileKind classifies a file found on a USB drive.
type FileKind string

const (
	KindEraseCertificate FileKind = "erase_certificate"
	KindTestReport       FileKind = "test_report"
	KindSpecSheet        FileKind = "spec_sheet"
	KindUnknown          FileKind = "unknown"
)

type FoundFile struct {
	Path string   `json:"path"`
	Kind FileKind `json:"kind"`
	// Report is set for KindEraseCertificate/KindTestReport.
	Report *toolstar.Report `json:"report,omitempty"`
}

// systemDirs are skipped while walking — Windows-created noise that's
// never going to hold a toolstar report, and System Volume Information in
// particular is usually not even readable by a normal user.
var systemDirs = map[string]bool{
	"System Volume Information": true,
	"$RECYCLE.BIN":              true,
}

// reportsDirName is toolstar's own output folder, sitting at the stick's
// root — shredderLX/testLX results can be anywhere inside it (nested by
// date, by run, ...). Scanning just this folder when it exists is both
// faster and more precise than walking the whole stick, which can hold a
// lot of unrelated content (e.g. a Ventoy stick's ISO storage).
const reportsDirName = "testlx"

// ScanDrive looks for .txt/.pdf files inside <root>/testlx if that folder
// exists (case-insensitively — Windows filesystems are case-preserving,
// not case-sensitive), anywhere inside it. Falls back to walking the whole
// drive when there's no such folder, for a stick that doesn't use
// toolstar's usual layout. Classifies each file found; PDF isn't parsed
// yet (see TODO in ClassifyFile) — such files are reported as unknown so
// the UI can still show they exist.
func ScanDrive(root string) ([]FoundFile, error) {
	scanRoot := root
	if info, err := os.Stat(filepath.Join(root, reportsDirName)); err == nil && info.IsDir() {
		scanRoot = filepath.Join(root, reportsDirName)
	}

	var found []FoundFile

	err := filepath.WalkDir(scanRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entry (permissions, ...) — skip, don't abort the whole scan
		}
		if entry.IsDir() {
			if systemDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".txt" && ext != ".pdf" {
			return nil
		}
		if ff, err := ClassifyFile(path); err == nil {
			found = append(found, ff)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return found, nil
}

func ClassifyFile(path string) (FoundFile, error) {
	ext := strings.ToLower(filepath.Ext(path))

	var content string
	switch ext {
	case ".pdf":
		text, err := extractPDFText(path)
		if err != nil {
			return FoundFile{Path: path, Kind: KindUnknown}, nil
		}
		content = text
	default:
		data, err := os.ReadFile(path)
		if err != nil {
			return FoundFile{}, err
		}
		content = string(data)
	}

	lower := strings.ToLower(filepath.Base(path))
	if strings.HasPrefix(lower, "pc_info") {
		return FoundFile{Path: path, Kind: KindSpecSheet}, nil
	}

	report := toolstar.ParseTXT(content)
	if report.IsEraseCertificate() {
		return FoundFile{Path: path, Kind: KindEraseCertificate, Report: report}, nil
	}
	if report.TestResult != "" || report.PCName != "" {
		return FoundFile{Path: path, Kind: KindTestReport, Report: report}, nil
	}
	return FoundFile{Path: path, Kind: KindUnknown}, nil
}

// extractPDFText pulls plain text out of a PDF using the same label-hunting
// parser as TXT reports — GLPI's attached shred-*/test* documents are PDFs
// (see project memory), and PDF text extraction tends to run adjacent table
// cells together with no separator, which toolstar.ParseTXT already handles
// (it was written to survive that from analyzing a real sample).
func extractPDFText(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var buf bytes.Buffer
	totalPage := r.NumPage()
	for i := 1; i <= totalPage; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(text)
		buf.WriteString("\n")
	}
	return buf.String(), nil
}
