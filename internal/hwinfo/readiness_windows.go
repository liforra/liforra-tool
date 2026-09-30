//go:build windows

package hwinfo

import (
	"context"
	"encoding/json"
	"os/exec"
)

// OSReadiness is OS-level "ready to hand off" signals — separate from
// Info/Detect() because a Windows Update search can take a while (and
// occasionally hang if it can't reach Microsoft), so it's only fetched
// from the device-check page, not on every scan.
type OSReadiness struct {
	// LocalAccounts lists every *enabled* local account, minus Windows'
	// own pseudo-accounts (DefaultAccount, WDAGUtilityAccount) and the
	// built-in Guest — a clean reset should leave exactly one, named
	// "User". Anything else (a leftover personal account, several
	// accounts, none at all) is worth a technician's attention.
	LocalAccounts []string `json:"localAccounts"`
	// PendingUpdates is -1 when the search itself couldn't be completed
	// (no network, WU service unavailable, timed out) — deliberately not
	// 0, so "couldn't check" is never shown as "all good".
	PendingUpdates int `json:"pendingUpdates"`
	// ProblemDevices names every PnP device Windows itself considers
	// broken (ConfigManagerErrorCode != 0) — missing/failed drivers,
	// resource conflicts, "unknown device", etc.: the same set Device
	// Manager marks with a yellow warning icon. Code 22 (deliberately
	// disabled by a user) is excluded — that's a choice, not a problem,
	// and plenty of machines have a disabled radio/adapter on purpose.
	ProblemDevices []string `json:"problemDevices"`
}

const readinessScript = `
$ErrorActionPreference = 'SilentlyContinue'
$excluded = @('DefaultAccount', 'WDAGUtilityAccount', 'Guest')
$localAccounts = @(Get-LocalUser | Where-Object { $_.Enabled -and ($_.Name -notin $excluded) } | Select-Object -ExpandProperty Name)

$pendingUpdates = -1
try {
    $session = New-Object -ComObject Microsoft.Update.Session
    $searcher = $session.CreateUpdateSearcher()
    $result = $searcher.Search("IsInstalled=0 and IsHidden=0 and Type='Software'")
    $pendingUpdates = $result.Updates.Count
} catch {
    $pendingUpdates = -1
}

$problemDevices = @(Get-CimInstance Win32_PnPEntity | Where-Object {
    $_.ConfigManagerErrorCode -ne 0 -and $_.ConfigManagerErrorCode -ne 22
} | ForEach-Object { "$($_.Name) (Code $($_.ConfigManagerErrorCode))" })

[PSCustomObject]@{
    localAccounts  = @($localAccounts)
    pendingUpdates = $pendingUpdates
    problemDevices = @($problemDevices)
} | ConvertTo-Json -Compress
`

// DetectOSReadiness runs the above — callers should bound ctx (a Windows
// Update search has no built-in timeout of its own).
func DetectOSReadiness(ctx context.Context) (*OSReadiness, error) {
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", readinessScript)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var r OSReadiness
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
