package main

import (
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"liforra-tool/internal/config"
	"liforra-tool/internal/localsettings"
	"liforra-tool/internal/updater"
)

// UpdateStatus is pushed to the frontend as an "update:status" event and
// also available via GetUpdateStatus() for a freshly-mounted UI.
type UpdateStatus struct {
	// State: "checking" | "none" | "downloading" | "outdated" | "error"
	State         string `json:"state"`
	NewerCount    int    `json:"newerCount"`
	LatestVersion string `json:"latestVersion,omitempty"`
	Error         string `json:"error,omitempty"`
}

func (a *App) updaterClient() *updater.Client {
	s, _ := localsettings.Load()
	return updater.NewClient(s.UpdateServerURL, config.DownloadToken)
}

func (a *App) emitUpdateStatus(s UpdateStatus) {
	a.updateStatus = s
	wailsruntime.EventsEmit(a.ctx, "update:status", s)
}

// runUpdateCheck compares this build's version against the server's list
// (newest first) — position in that list IS the "how many versions ahead"
// count, so this works with whatever versioning scheme the server uses,
// not just strict semver. If auto-update is on and a newer version exists,
// it's staged in the background (see internal/updater) rather than just
// reported.
func (a *App) runUpdateCheck() {
	a.emitUpdateStatus(UpdateStatus{State: "checking"})

	client := a.updaterClient()
	versions, err := client.ListVersions(a.ctx)
	if err != nil {
		a.emitUpdateStatus(UpdateStatus{State: "error", Error: err.Error()})
		return
	}
	if len(versions) == 0 {
		a.emitUpdateStatus(UpdateStatus{State: "none"})
		return
	}

	idx := -1
	for i, v := range versions {
		if v.Version == config.AppVersion {
			idx = i
			break
		}
	}
	// idx == 0: we're already the latest. idx == -1: current version isn't
	// in the list (e.g. a local "dev" build) — nothing sensible to report.
	if idx <= 0 {
		a.emitUpdateStatus(UpdateStatus{State: "none"})
		return
	}

	newerCount := idx
	latest := versions[0]

	settings, _ := localsettings.Load()
	if !settings.AutoUpdate {
		a.emitUpdateStatus(UpdateStatus{State: "outdated", NewerCount: newerCount, LatestVersion: latest.Version})
		return
	}

	a.emitUpdateStatus(UpdateStatus{State: "downloading", NewerCount: newerCount, LatestVersion: latest.Version})
	if err := updater.StageVersion(a.ctx, client, latest); err != nil {
		a.emitUpdateStatus(UpdateStatus{State: "error", NewerCount: newerCount, LatestVersion: latest.Version, Error: err.Error()})
		return
	}
	// Staged successfully — the next launch will be the new version. Not
	// "outdated" any more from the user's perspective, since there's
	// nothing left for them to do.
	a.emitUpdateStatus(UpdateStatus{State: "none"})
}

// GetUpdateStatus returns the last known status, for a UI that mounts after
// the startup check already ran (it also gets live updates via the
// "update:status" event).
func (a *App) GetUpdateStatus() UpdateStatus {
	return a.updateStatus
}

// CheckForUpdatesNow re-runs the check on demand (e.g. a manual refresh in
// the version browser).
func (a *App) CheckForUpdatesNow() {
	go a.runUpdateCheck()
}

func (a *App) ListAvailableVersions() ([]updater.VersionInfo, error) {
	return a.updaterClient().ListVersions(a.ctx)
}

// InstallVersion stages a specific version — upgrade or downgrade, picked
// by the user in the version browser. Applies on next launch, same as an
// automatic update.
func (a *App) InstallVersion(version string) error {
	client := a.updaterClient()
	detail, err := client.VersionDetail(a.ctx, version)
	if err != nil {
		return err
	}
	return updater.StageVersion(a.ctx, client, *detail)
}

func (a *App) GetAutoUpdate() bool {
	s, _ := localsettings.Load()
	return s.AutoUpdate
}

func (a *App) SetAutoUpdate(enabled bool) error {
	s, _ := localsettings.Load()
	s.AutoUpdate = enabled
	return localsettings.Save(s)
}

func (a *App) GetUpdateServerURL() string {
	s, _ := localsettings.Load()
	return s.UpdateServerURL
}

func (a *App) SetUpdateServerURL(url string) error {
	s, _ := localsettings.Load()
	s.UpdateServerURL = url
	return localsettings.Save(s)
}

func (a *App) GetAppVersion() string {
	return config.AppVersion
}

func (a *App) GetAppName() string {
	return config.AppName
}
