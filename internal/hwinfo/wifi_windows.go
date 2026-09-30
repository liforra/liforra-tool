//go:build windows

package hwinfo

import (
	"context"
	"encoding/json"
	"os/exec"
)

type WiFiCleanupResult struct {
	Removed int `json:"removed"`
	// Failed lists any profile netsh itself refused to delete — one
	// failure doesn't abort the rest.
	Failed []string `json:"failed"`
}

// wifiCleanupScript parses "netsh wlan show profiles" by structure, not by
// its label text — that text is localized ("All User Profile" in English,
// "Profil für alle Benutzer" in German, confirmed live 2026-09-29 on a
// German-locale machine), but every real entry is indented and has the
// form "<label> : <name>", while every section header/separator/placeholder
// line either has no colon or has nothing after it. So: any indented line
// with a non-empty value after its first colon is a profile name,
// regardless of Windows' display language.
const wifiCleanupScript = `
$ErrorActionPreference = 'SilentlyContinue'
$names = @()
foreach ($line in (netsh wlan show profiles)) {
    if ($line -match '^\s+\S.*:\s*(\S.*)$') { $names += $matches[1].Trim() }
}
$names = $names | Select-Object -Unique

$removed = 0
$failed = @()
foreach ($n in $names) {
    netsh wlan delete profile name="$n" | Out-Null
    if ($LASTEXITCODE -eq 0) { $removed++ } else { $failed += $n }
}

[PSCustomObject]@{ removed = $removed; failed = @($failed) } | ConvertTo-Json -Compress
`

// RemoveAllWiFiProfiles deletes every saved WLAN profile on this machine,
// passwords included. A hand-off action, not a check — call only when a
// technician has explicitly confirmed it (see the device-check page's
// confirmation dialog), never automatically, and never as part of
// DetectOSReadiness. Irreversible.
func RemoveAllWiFiProfiles(ctx context.Context) (*WiFiCleanupResult, error) {
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", wifiCleanupScript)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var r WiFiCleanupResult
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
