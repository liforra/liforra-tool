package main

import (
	"context"
	"fmt"
	"time"

	"liforra-tool/internal/devicecheck"
	"liforra-tool/internal/glpi"
	"liforra-tool/internal/hwinfo"
	"liforra-tool/internal/usbscan"
)

// CheckDeviceDocuments cross-checks the live hardware scan against whatever
// erase certificate / test report were found on the USB stick — no GLPI
// involved, safe to call before a device even exists there. Pure, so it
// can't fail.
func (a *App) CheckDeviceDocuments(info hwinfo.Info, files []usbscan.FoundFile) []devicecheck.Result {
	return devicecheck.CheckDocuments(info, files)
}

// CheckDeviceAgainstGLPI compares matches (as returned by
// FindComputerBySerial — the frontend already has to call that to get the
// matched Computer's id for MarkDeviceChecked, so this doesn't repeat the
// lookup) against what's physically detected. Pure.
func (a *App) CheckDeviceAgainstGLPI(info hwinfo.Info, matches []glpi.Computer) []devicecheck.Result {
	return devicecheck.CheckGLPI(info, matches)
}

// CheckDeviceOS checks OS-level readiness (local accounts, pending
// updates, devices with a broken/missing driver) — separate from the
// hardware scan because a Windows Update search can be slow; bounded so a
// stuck search can't hang the check page.
func (a *App) CheckDeviceOS() ([]devicecheck.Result, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	defer cancel()
	r, err := hwinfo.DetectOSReadiness(ctx)
	if err != nil {
		return nil, fmt.Errorf("Systemprüfung fehlgeschlagen: %w", err)
	}
	return devicecheck.CheckOS(*r), nil
}

// RemoveAllWiFiNetworks deletes every saved WLAN profile on this machine,
// passwords included — the device-check page's cleanup button before
// handing a refurbished device off. Irreversible; the frontend confirms
// with the technician before calling this.
func (a *App) RemoveAllWiFiNetworks() (*hwinfo.WiFiCleanupResult, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	r, err := hwinfo.RemoveAllWiFiProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("WLAN-Netzwerke konnten nicht entfernt werden: %w", err)
	}
	return r, nil
}

// groupNameContains/locationNameContains are fixed values this app itself
// specifies (not typed in by the technician) — matched by substring, not
// exact spelling, since these are casual names, not necessarily GLPI's
// exact stored one. See glpi.Client.ResolveByNameContains.
//
const (
	groupNameContains    = "azubi"
	locationNameContains = "akademie"
)

// MarkDeviceChecked sets "Verantwortlicher Techniker" (whoever is logged
// in right now), "Verantwortliche Gruppe" and "Standort" — the
// device-check page's "Fertig geprüft" button. A write; not exercised live
// during development, per [[glpi_write_approval_rule]] in project memory:
// built from the same documented v1 conventions as internal/glpi/write.go's
// other calls, plus real field names/types confirmed live 2026-09-29
// against ~50 real Computer records (see write.go's ZusatzdatenUpdate).
func (a *App) MarkDeviceChecked(computerID int) error {
	if err := a.requireV1Session(); err != nil {
		return err
	}
	if a.session.UserID == 0 {
		id, err := a.glpi.GetCurrentUserID(a.ctx, a.session)
		if err != nil {
			return fmt.Errorf("eigene Benutzer-ID konnte nicht ermittelt werden: %w", err)
		}
		a.session.UserID = id
	}
	if a.groupIDTech == 0 {
		id, _, err := a.glpi.ResolveByNameContains(a.ctx, a.session, "Group", groupNameContains)
		if err != nil {
			return fmt.Errorf("Gruppe konnte nicht ermittelt werden: %w", err)
		}
		a.groupIDTech = id
	}
	if a.locationID == 0 && locationNameContains != "" {
		id, _, err := a.glpi.ResolveByNameContains(a.ctx, a.session, "Location", locationNameContains)
		if err != nil {
			return fmt.Errorf("Standort konnte nicht ermittelt werden: %w", err)
		}
		a.locationID = id
	}
	if err := a.glpi.SetResponsibleFields(a.ctx, a.session, computerID, a.session.UserID, a.groupIDTech, a.locationID); err != nil {
		return fmt.Errorf("Techniker/Gruppe/Standort konnten nicht gesetzt werden: %w", err)
	}
	return nil
}
