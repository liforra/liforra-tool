package buildserver

import (
	"html/template"
	"net/http"
	"strings"
)

// The whole site is gated behind the same download token as the API (see
// server.go's withToken) — reachable at all only with a token a compiled
// app itself hands out (see about.go's GetAboutInfo), by design: a random
// visitor without one can't even see the version list, let alone download
// anything.
const pageShell = `<!DOCTYPE html>
<html lang="de">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — liforra-tool</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@400;500;700&family=IBM+Plex+Mono:wght@500&display=swap" rel="stylesheet">
<style>
  :root {
    --plum-950: #180f22; --plum-900: #1f1430; --plum-800: #2c1e42; --plum-600: #543a72;
    --lilac-300: #c9b8e0; --lilac-100: #ede6f5; --paper-50: #faf7f2;
    --signal-500: #9d5cf0; --signal-400: #b47cf5; --danger-400: #f0857c;
    --font-display: 'Space Grotesk', system-ui, sans-serif;
    --font-tag: 'IBM Plex Mono', ui-monospace, monospace;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; min-height: 100vh; background: var(--plum-950); color: var(--lilac-100);
    font-family: var(--font-display); padding: 2rem 1rem 4rem;
  }
  .wrap { max-width: 720px; margin: 0 auto; }
  header { display: flex; align-items: baseline; justify-content: space-between; margin-bottom: 2rem; flex-wrap: wrap; gap: 0.5rem; }
  .brand { font-family: var(--font-tag); font-size: 0.85rem; color: var(--signal-400); letter-spacing: 0.02em; }
  h1 { font-size: 1.6rem; margin: 0.2rem 0 0; }
  nav a { color: var(--lilac-300); text-decoration: none; font-size: 0.85rem; margin-left: 1.2rem; }
  nav a:hover { color: var(--signal-400); }
  .card {
    background: var(--plum-900); border: 1px solid var(--plum-800); border-radius: 12px;
    padding: 1.2rem 1.4rem; margin-bottom: 1rem;
  }
  .version-row { display: flex; align-items: center; justify-content: space-between; gap: 1rem; flex-wrap: wrap; }
  .version-row + .version-row { border-top: 1px solid var(--plum-800); margin-top: 0.9rem; padding-top: 0.9rem; }
  .version-name { font-family: var(--font-tag); font-size: 1rem; }
  .version-date { color: var(--lilac-300); font-size: 0.8rem; }
  .changelog { color: var(--lilac-300); font-size: 0.85rem; margin: 0.4rem 0 0; white-space: pre-wrap; }
  .links { display: flex; gap: 0.5rem; flex-wrap: wrap; }
  .btn {
    display: inline-block; background: var(--signal-500); color: var(--paper-50); text-decoration: none;
    font-size: 0.82rem; padding: 0.4rem 0.8rem; border-radius: 999px; white-space: nowrap;
  }
  .btn:hover { background: var(--signal-400); }
  .btn--ghost { background: transparent; border: 1px solid var(--plum-600); color: var(--lilac-100); }
  .muted { color: var(--lilac-300); font-size: 0.85rem; }
  footer { margin-top: 2.5rem; color: var(--plum-600); font-size: 0.78rem; text-align: center; }
</style>
</head>
<body>
<div class="wrap">
  <header>
    <div>
      <div class="brand">liforra-tool</div>
      <h1>{{.Title}}</h1>
    </div>
    <nav>
      <a href="/?token={{.Token}}">Versionen</a>
      <a href="/install?token={{.Token}}">Installieren</a>
    </nav>
  </header>
  {{.Body}}
  <footer>liforra.de — privater Download-Server</footer>
</div>
</body>
</html>`

var shellTmpl = template.Must(template.New("shell").Parse(pageShell))

// body is pre-rendered HTML (from homeTmpl/installTmpl, already
// html/template-escaped there), so it's passed through verbatim via
// template.HTML rather than re-escaped as plain text.
func renderPage(w http.ResponseWriter, title, token string, body template.HTML) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = shellTmpl.Execute(w, struct {
		Title string
		Token string
		Body  template.HTML
	}{title, token, body})
}

const homeBody = `
{{if not .Versions}}
  <div class="card"><p class="muted">Noch keine Version veröffentlicht.</p></div>
{{end}}
{{range .Versions}}
  <div class="card">
    <div class="version-row">
      <div>
        <div class="version-name">{{.Version}}{{if eq .Version $.Latest}} <span class="muted">(aktuell)</span>{{end}}</div>
        <div class="version-date">{{.PublishedAt.Format "02.01.2006 15:04"}}</div>
      </div>
      <div class="links">
        {{if .HasZip}}<a class="btn" href="/api/versions/{{.Version}}/download.zip?token={{$.Token}}">ZIP</a>{{end}}
        {{if .HasInstaller}}<a class="btn" href="/api/versions/{{.Version}}/download-installer.exe?token={{$.Token}}">Installer</a>{{end}}
        <a class="btn btn--ghost" href="/api/versions/{{.Version}}/download?token={{$.Token}}">EXE</a>
      </div>
    </div>
    {{if .Changelog}}<p class="changelog">{{.Changelog}}</p>{{end}}
  </div>
{{end}}
`

var homeTmpl = template.Must(template.New("home").Parse(homeBody))

func (s *Server) handleWebsiteHome(w http.ResponseWriter, r *http.Request) {
	versions, err := s.Store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	token := r.URL.Query().Get("token")
	latest := ""
	if len(versions) > 0 {
		latest = versions[0].Version
	}

	var buf strings.Builder
	err = homeTmpl.Execute(&buf, struct {
		Versions []Meta
		Token    string
		Latest   string
	}{versions, token, latest})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderPage(w, "Versionen", token, template.HTML(buf.String()))
}

const installBody = `
<div class="card">
  <p>Ein eigenes Installationsprogramm (USB-Stick oder System, inkl. <span class="muted">install.exe</span>/<span class="muted">uninstall.exe</span>) ist noch nicht gebaut.</p>
  <p class="muted">Bis dahin: die EXE oben direkt herunterladen und ausführen.</p>
</div>
`

var installTmpl = template.Must(template.New("install").Parse(installBody))

func (s *Server) handleWebsiteInstall(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	var buf strings.Builder
	_ = installTmpl.Execute(&buf, nil)
	renderPage(w, "Installieren", token, template.HTML(buf.String()))
}
