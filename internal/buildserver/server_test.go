package buildserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"liforra-tool/internal/updater"
)

func newTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: store, DownloadToken: "download-tok", PublishToken: "publish-tok"}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts, srv
}

func publishMultipart(t *testing.T, ts *httptest.Server, version string, exeData []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("exe", "liforra-tool.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(exeData); err != nil {
		t.Fatal(err)
	}
	_ = w.WriteField("changelog", "test release")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/admin/versions/"+version, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer publish-tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish status %d", resp.StatusCode)
	}
}

// This is the test updater.go's own doc comment says never happened: the
// real client this app ships, driven against a real (in-process) instance
// of this server — proves the two sides actually agree on the wire
// format, not just that each looks right in isolation.
func TestRealUpdaterClient_AgainstThisServer(t *testing.T) {
	ts, _ := newTestServer(t)
	publishMultipart(t, ts, "1.0.0", []byte("exe contents v1"))
	publishMultipart(t, ts, "1.1.0", []byte("exe contents v1.1, a bit longer"))

	client := updater.NewClient(ts.URL, "download-tok")
	ctx := context.Background()

	versions, err := client.ListVersions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("got %d versions, want 2", len(versions))
	}
	if versions[0].Version != "1.1.0" {
		t.Errorf("versions[0] = %q, want 1.1.0 (newest first)", versions[0].Version)
	}

	latest, err := client.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Version != "1.1.0" {
		t.Errorf("Latest() = %q, want 1.1.0", latest.Version)
	}

	detail, err := client.VersionDetail(ctx, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Changelog != "test release" {
		t.Errorf("Changelog = %q", detail.Changelog)
	}

	// updater.StageVersion resolves os.Executable() itself and renames the
	// running exe aside — not safe to exercise directly against this test
	// binary (it would rename itself mid-run). Verify what it depends on
	// instead: the raw download round trip and that the served bytes hash
	// to exactly what the server's own metadata claims.
	resp, err := http.Get(ts.URL + "/api/versions/1.0.0/download?token=download-tok")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download status %d", resp.StatusCode)
	}
	downloaded, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(downloaded)
	if hex.EncodeToString(sum[:]) != detail.BinaryHashSHA256 {
		t.Error("downloaded bytes don't hash to what VersionDetail claimed")
	}
}

func TestAuth_MissingOrWrongTokenIsRejected(t *testing.T) {
	ts, _ := newTestServer(t)
	publishMultipart(t, ts, "1.0.0", []byte("x"))

	cases := []struct {
		name    string
		mutate  func(*http.Request)
		wantOK  bool
	}{
		{"no token", func(r *http.Request) {}, false},
		{"wrong bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") }, false},
		{"wrong query", func(r *http.Request) { q := r.URL.Query(); q.Set("token", "wrong"); r.URL.RawQuery = q.Encode() }, false},
		{"correct bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer download-tok") }, true},
		{"correct query", func(r *http.Request) { q := r.URL.Query(); q.Set("token", "download-tok"); r.URL.RawQuery = q.Encode() }, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/versions", nil)
			c.mutate(req)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			ok := resp.StatusCode == http.StatusOK
			if ok != c.wantOK {
				t.Errorf("status %d, wantOK=%v", resp.StatusCode, c.wantOK)
			}
		})
	}
}

func TestAuth_PublishTokenDoesNotGrantDownloadAccess(t *testing.T) {
	ts, _ := newTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/versions", nil)
	req.Header.Set("Authorization", "Bearer publish-tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("the higher-privilege publish token must not also work as a download token")
	}
}

func TestAuth_DownloadTokenDoesNotGrantPublishAccess(t *testing.T) {
	ts, _ := newTestServer(t)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("exe", "x.exe")
	_, _ = fw.Write([]byte("x"))
	_ = w.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/admin/versions/1.0.0", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer download-tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("the low-privilege download token must not also work for publishing")
	}
}

func TestWebsite_RequiresToken(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("website home must not be reachable without a token")
	}

	resp2, err := http.Get(ts.URL + "/?token=download-tok")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("website home with a valid token = %d, want 200", resp2.StatusCode)
	}
}
