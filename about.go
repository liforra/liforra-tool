package main

import (
	"net/url"

	"liforra-tool/internal/config"
	"liforra-tool/internal/localsettings"
)

// AboutInfo is everything the About screen shows: the app's identity plus a
// ready-to-use download link — the build server's website with this
// install's download token already attached, so anyone with the compiled
// app can reach a working download link without ever having the token
// itself surfaced to them anywhere else. See internal/config.DownloadToken.
type AboutInfo struct {
	AppName     string `json:"appName"`
	AppVersion  string `json:"appVersion"`
	CompanyName string `json:"companyName"`
	// DownloadURL is empty when this build has no token (a local "dev"
	// build) — there is no working link to show.
	DownloadURL   string `json:"downloadUrl"`
	SourceCodeURL string `json:"sourceCodeUrl"`
	License       string `json:"license"`
}

// GetAboutInfo backs the About screen (Menü → Über).
func (a *App) GetAboutInfo() AboutInfo {
	s, _ := localsettings.Load()

	info := AboutInfo{
		AppName:       config.AppName,
		AppVersion:    config.AppVersion,
		CompanyName:   "liforra.de",
		SourceCodeURL: "https://github.com/liforra/liforra-tool",
		License:       "AGPL-3.0",
	}
	if config.DownloadToken != "" {
		base := s.UpdateServerURL
		info.DownloadURL = base + "/?token=" + url.QueryEscape(config.DownloadToken)
	}
	return info
}
