// Package updater talks to the private lumatool update server and, when
// auto-update is on, stages a newer version to take over on next launch.
//
// Self-replace approach: Windows won't let a running exe be deleted or
// overwritten in place, but it *does* allow renaming one while it's
// running (the process keeps executing from the already-mapped file). So
// staging an update means: download the new binary next to the current
// one, rename the running exe to "<name>.old", then rename the download
// into the running exe's original name. The current process keeps running
// unaffected; the *next* launch (a fresh double-click / shortcut) picks up
// the new binary. On next startup, CleanupOldBinary removes the ".old"
// leftover.
//
// Not yet exercised against a real server (none is deployed yet) — the API
// shape here is this project's own design, not a discovered/confirmed
// contract, so keep client and server in sync if either changes.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type VersionInfo struct {
	Version          string    `json:"version"`
	PublishedAt      time.Time `json:"publishedAt"`
	Changelog        string    `json:"changelog"`
	ConfigChanged    bool      `json:"configChanged"`
	BinaryHashSHA256 string    `json:"binaryHashSHA256"`
	SizeBytes        int64     `json:"sizeBytes"`
}

type Client struct {
	ServerURL string
	Token     string
	HTTP      *http.Client
}

func NewClient(serverURL, token string) *Client {
	return &Client{ServerURL: serverURL, Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ServerURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	return c.HTTP.Do(req)
}

func (c *Client) ListVersions(ctx context.Context) ([]VersionInfo, error) {
	resp, err := c.get(ctx, "/api/versions")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var versions []VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, err
	}
	return versions, nil
}

func (c *Client) Latest(ctx context.Context) (*VersionInfo, error) {
	resp, err := c.get(ctx, "/api/versions/latest")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var v VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) VersionDetail(ctx context.Context, version string) (*VersionInfo, error) {
	resp, err := c.get(ctx, "/api/versions/"+version)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var v VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	return &v, nil
}

// StageVersion downloads the given version next to the running exe,
// verifies its hash, and swaps it into place for the next launch (current
// process keeps running the old code). See package doc for why this is
// safe on Windows.
func StageVersion(ctx context.Context, c *Client, v VersionInfo) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return err
	}

	resp, err := c.get(ctx, "/api/versions/"+v.Version+"/download")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download status %d: %s", resp.StatusCode, string(body))
	}

	newPath := exePath + ".new"
	f, err := os.Create(newPath)
	if err != nil {
		return err
	}

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, hasher), resp.Body); err != nil {
		f.Close()
		os.Remove(newPath)
		return err
	}
	f.Close()

	gotHash := hex.EncodeToString(hasher.Sum(nil))
	if gotHash != v.BinaryHashSHA256 {
		os.Remove(newPath)
		return fmt.Errorf("hash mismatch: got %s, server said %s — download not applied", gotHash, v.BinaryHashSHA256)
	}

	oldPath := exePath + ".old"
	_ = os.Remove(oldPath) // best-effort cleanup of any previous leftover
	if err := os.Rename(exePath, oldPath); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("could not rename running exe aside: %w", err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		// Try to put the original back so the app isn't left broken.
		_ = os.Rename(oldPath, exePath)
		return fmt.Errorf("could not move new version into place: %w", err)
	}
	return nil
}

// CleanupOldBinary removes a ".old" leftover from a previous update, if
// any. Call once at startup; errors are non-fatal (the file might still be
// briefly locked right after the old process exits).
func CleanupOldBinary() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	_ = os.Remove(exePath + ".old")
}
