package buildserver

import (
	"archive/zip"
	"bytes"
)

// buildZip bundles a version's files into the "zip download" — the exe
// alone for now. installerData is accepted (and would be included
// alongside as install.exe once that tool exists) but unused today since
// there's nothing built yet to add — see project chat: install.exe/
// uninstall.exe are still pending work.
func buildZip(version string, exeData []byte, installerData []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	f, err := w.Create("liforra-tool.exe")
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(exeData); err != nil {
		return nil, err
	}

	if len(installerData) > 0 {
		f, err := w.Create("install.exe")
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(installerData); err != nil {
			return nil, err
		}
	}

	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
