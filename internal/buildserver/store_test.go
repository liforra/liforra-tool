package buildserver

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
)

func TestPublishAndGet(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	exeData := []byte("fake exe bytes for version 1.2.3")
	m, err := s.Publish("1.2.3", exeData, nil, "first release", false)
	if err != nil {
		t.Fatal(err)
	}

	wantHash := sha256.Sum256(exeData)
	if m.BinaryHashSHA256 != hex.EncodeToString(wantHash[:]) {
		t.Errorf("BinaryHashSHA256 = %q, want %q", m.BinaryHashSHA256, hex.EncodeToString(wantHash[:]))
	}
	if m.SizeBytes != int64(len(exeData)) {
		t.Errorf("SizeBytes = %d, want %d", m.SizeBytes, len(exeData))
	}
	if !m.HasZip {
		t.Error("expected HasZip = true")
	}
	if m.HasInstaller {
		t.Error("expected HasInstaller = false (no installer given)")
	}

	got, err := s.Get("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.2.3" || got.Changelog != "first release" {
		t.Errorf("Get() = %+v", got)
	}
}

func TestGet_NotFoundIsErrNotFound(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("nope"); err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestList_NewestFirst(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"1.0.0", "1.1.0", "1.2.0"} {
		if _, err := s.Publish(v, []byte("exe-"+v), nil, "", false); err != nil {
			t.Fatal(err)
		}
	}

	versions, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 {
		t.Fatalf("got %d versions, want 3", len(versions))
	}
	// Publish() stamps PublishedAt with time.Now() in call order, so the
	// last one published (1.2.0) must sort first.
	if versions[0].Version != "1.2.0" {
		t.Errorf("versions[0] = %q, want 1.2.0 (%+v)", versions[0].Version, versions)
	}
}

func TestPublish_ZipContainsTheExe(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exeData := []byte("the real exe content")
	m, err := s.Publish("2.0.0", exeData, nil, "", false)
	if err != nil {
		t.Fatal(err)
	}

	zr, err := zip.OpenReader(s.ZipPath("2.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 1 || zr.File[0].Name != "liforra-tool.exe" {
		t.Fatalf("zip contents = %+v", zr.File)
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, exeData) {
		t.Error("zip's liforra-tool.exe doesn't match the published exe bytes")
	}
	if !m.HasZip || m.ZipHashSHA256 == "" {
		t.Errorf("zip metadata not set: %+v", m)
	}
}

func TestDelete_RemovesVersion(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish("3.0.0", []byte("x"), nil, "", false); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("3.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("3.0.0"); err != ErrNotFound {
		t.Errorf("err after delete = %v, want ErrNotFound", err)
	}
}

func TestPublish_OverwritesSameVersion(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish("1.0.0", []byte("old"), nil, "old changelog", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish("1.0.0", []byte("new"), nil, "new changelog", true); err != nil {
		t.Fatal(err)
	}
	m, err := s.Get("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if m.Changelog != "new changelog" || !m.ConfigChanged {
		t.Errorf("overwrite didn't take: %+v", m)
	}
	versions, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Errorf("got %d versions after overwrite, want 1 (no duplicate)", len(versions))
	}
}
