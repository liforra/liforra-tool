package buildserver

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// Server wires a Store to HTTP handlers matching internal/updater's
// client exactly (see that package's doc), plus a publish/admin API and
// the public download website. DownloadToken is the one every compiled
// app embeds (config.DownloadToken) — accepted as either a Bearer header
// (the app's own API calls) or a ?token= query parameter (a plain link a
// human clicks, e.g. from the About page — a browser can't set a custom
// header on a plain link click). PublishToken is a separate, higher-
// privilege credential for releasing a new version; it never ships in any
// client and is only ever checked as a Bearer header.
type Server struct {
	Store         *Store
	DownloadToken string
	PublishToken  string
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/versions", s.withToken(s.DownloadToken, s.handleListVersions))
	mux.HandleFunc("GET /api/versions/latest", s.withToken(s.DownloadToken, s.handleLatest))
	mux.HandleFunc("GET /api/versions/{version}", s.withToken(s.DownloadToken, s.handleVersionDetail))
	mux.HandleFunc("GET /api/versions/{version}/download", s.withToken(s.DownloadToken, s.handleDownloadExe))
	mux.HandleFunc("GET /api/versions/{version}/download.zip", s.withToken(s.DownloadToken, s.handleDownloadZip))
	mux.HandleFunc("GET /api/versions/{version}/download-installer.exe", s.withToken(s.DownloadToken, s.handleDownloadInstaller))

	mux.HandleFunc("POST /admin/versions/{version}", s.withToken(s.PublishToken, s.handlePublish))
	mux.HandleFunc("DELETE /admin/versions/{version}", s.withToken(s.PublishToken, s.handleDelete))

	mux.HandleFunc("GET /", s.withToken(s.DownloadToken, s.handleWebsiteHome))
	mux.HandleFunc("GET /install", s.withToken(s.DownloadToken, s.handleWebsiteInstall))

	return mux
}

// withToken gates a handler behind one of the two tokens above. A missing/
// empty configured token always refuses — a misconfigured server should
// fail closed, never fail open to "anyone gets in".
func (s *Server) withToken(want string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if want == "" || !validToken(r, want) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func validToken(r *http.Request, want string) bool {
	if tok := r.URL.Query().Get("token"); tok != "" {
		return subtle.ConstantTimeCompare([]byte(tok), []byte(want)) == 1
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		got := strings.TrimPrefix(auth, "Bearer ")
		return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
	}
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := s.Store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if versions == nil {
		versions = []Meta{}
	}
	writeJSON(w, versions)
}

func (s *Server) handleLatest(w http.ResponseWriter, r *http.Request) {
	versions, err := s.Store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(versions) == 0 {
		http.Error(w, "no versions published", http.StatusNotFound)
		return
	}
	writeJSON(w, versions[0])
}

func (s *Server) handleVersionDetail(w http.ResponseWriter, r *http.Request) {
	m, err := s.Store.Get(r.PathValue("version"))
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, m)
}

func (s *Server) handleDownloadExe(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	if _, err := s.Store.Get(version); errors.Is(err, ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="liforra-tool.exe"`)
	http.ServeFile(w, r, s.Store.ExePath(version))
}

func (s *Server) handleDownloadZip(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	m, err := s.Store.Get(version)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !m.HasZip {
		http.Error(w, "no zip for this version", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="liforra-tool-`+version+`.zip"`)
	http.ServeFile(w, r, s.Store.ZipPath(version))
}

func (s *Server) handleDownloadInstaller(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	m, err := s.Store.Get(version)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !m.HasInstaller {
		http.Error(w, "no installer for this version", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="liforra-tool-`+version+`-installer.exe"`)
	http.ServeFile(w, r, s.Store.InstallerPath(version))
}

// handlePublish accepts a multipart form: "exe" (required file field),
// "installer" (optional file field — install.exe, once that tool exists),
// "changelog" (string field), "configChanged" (string field, "true"/"false").
func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	if err := r.ParseMultipartForm(200 << 20); err != nil {
		http.Error(w, "invalid form: "+err.Error(), http.StatusBadRequest)
		return
	}

	exeFile, _, err := r.FormFile("exe")
	if err != nil {
		http.Error(w, "missing \"exe\" file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer exeFile.Close()
	exeData, err := io.ReadAll(exeFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var installerData []byte
	if f, _, err := r.FormFile("installer"); err == nil {
		defer f.Close()
		installerData, _ = io.ReadAll(f)
	}

	m, err := s.Store.Publish(version, exeData, installerData, r.FormValue("changelog"), r.FormValue("configChanged") == "true")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, m)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Delete(r.PathValue("version")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
