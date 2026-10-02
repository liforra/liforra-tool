package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"liforra-tool/internal/config"
	"liforra-tool/internal/glpi"
	"liforra-tool/internal/hwinfo"
	"liforra-tool/internal/localsettings"
	"liforra-tool/internal/specsheet"
	"liforra-tool/internal/updater"
	"liforra-tool/internal/usbscan"
)

// App struct
type App struct {
	ctx          context.Context
	glpi         *glpi.Client
	session      *glpi.Session
	updateStatus UpdateStatus
	// groupIDTech/locationID cache MarkDeviceChecked's two fixed-value
	// lookups (see devicecheck.go) for the lifetime of the run, same idea
	// as session.UserID.
	groupIDTech int
	locationID  int
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		glpi: glpi.NewClient(config.GLPIBaseURL, config.OAuthClientID, config.OAuthClientSecret, config.V1AppToken),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	updater.CleanupOldBinary()
	go a.runUpdateCheck()
}

// LoginResult is what the frontend gets back after attempting a login.
// Never includes the actual tokens — the frontend just needs to know whether
// it worked, since the session is kept entirely on the Go side.
type LoginResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	AsUser  string `json:"asUser,omitempty"`
	// LimitedAccess is true when the session resumed with v2 (read) access
	// but failed to restore its v1 session — every write (creating/editing
	// a device, checking GLPI's catalogs) needs v1 and will fail with no
	// visible reason until the technician logs in again with their
	// password. See TryResumeSession.
	LimitedAccess bool `json:"limitedAccess,omitempty"`
}

// Login authenticates against GLPI with the given technician credentials.
// The password only ever exists in memory for the duration of this call.
// On success, the login is persisted (encrypted, see internal/localsettings)
// so the technician doesn't have to log in again for a few days.
func (a *App) Login(username, password string) LoginResult {
	sess, err := a.glpi.Login(a.ctx, username, password)
	if err != nil {
		return LoginResult{Success: false, Error: humanizeLoginError(err)}
	}
	a.session = sess

	if err := localsettings.SaveCredentials(localsettings.Credentials{
		Username:     username,
		Password:     password,
		RefreshToken: sess.RefreshToken,
	}); err != nil {
		// Not fatal — the technician is still logged in for this run,
		// they'd just need to log in again next time.
		fmt.Println("warning: could not persist session:", err)
	}

	return LoginResult{Success: true, AsUser: username}
}

// TryResumeSession silently re-authenticates using a persisted session, if
// there is one — both the v2 OAuth side (via the refresh token) and the v1
// side (via the persisted password, since v1 sessions can't be refreshed).
// Call this once at startup before falling back to showing the login screen.
func (a *App) TryResumeSession() LoginResult {
	saved, err := localsettings.LoadCredentials()
	if err != nil || saved == nil || saved.RefreshToken == "" {
		return LoginResult{Success: false}
	}

	sess := &glpi.Session{RefreshToken: saved.RefreshToken, Username: saved.Username}
	if err := a.glpi.Refresh(a.ctx, sess); err != nil {
		_ = localsettings.ClearCredentials()
		return LoginResult{Success: false}
	}

	v1OK := false
	if saved.Password != "" {
		if v1tok, err := a.glpi.InitV1Session(a.ctx, saved.Username, saved.Password); err == nil {
			sess.V1SessionToken = v1tok
			v1OK = true
		}
		// If this fails (e.g. password changed GLPI-side), we still have
		// v2 access — just not v1 — rather than failing the whole resume.
		// This used to be silent: the technician stayed "logged in" with
		// every write failing for no visible reason (every create/edit/
		// check-GLPI call requires v1) until they happened to log out and
		// back in. LimitedAccess below is what fixes that.
	}

	a.session = sess

	// Refresh tokens can rotate — persist whatever we now have.
	_ = localsettings.SaveCredentials(localsettings.Credentials{
		Username:     saved.Username,
		Password:     saved.Password,
		RefreshToken: sess.RefreshToken,
	})

	return LoginResult{Success: true, AsUser: saved.Username, LimitedAccess: !v1OK}
}

// IsLoggedIn reports whether a session is currently held.
func (a *App) IsLoggedIn() bool {
	return a.session != nil
}

// Logout drops the in-memory session and any persisted one.
func (a *App) Logout() {
	a.session = nil
	_ = localsettings.ClearCredentials()
}

// ListUSBDrives returns the currently mounted removable drives.
func (a *App) ListUSBDrives() ([]usbscan.Drive, error) {
	return usbscan.ListRemovableDrives()
}

// ScanUSBDrive looks for toolstar reports and a spec sheet on the given
// drive path (as returned by ListUSBDrives).
func (a *App) ScanUSBDrive(path string) ([]usbscan.FoundFile, error) {
	return usbscan.ScanDrive(path)
}

// EjectDrive safely ejects a removable drive so a technician can pull the
// USB stick right after, without risking a half-written file.
func (a *App) EjectDrive(path string) error {
	return usbscan.EjectDrive(path)
}

// GetActiveProfile returns the name of the currently active GLPI profile
// (e.g. "Self-Service", "Technician") for display — see
// glpi.GetActiveProfileName for why this matters: a technician with
// multiple GLPI profiles may be logged into one without asset-write rights
// without any other indication of why writes are failing.
func (a *App) GetActiveProfile() (string, error) {
	if err := a.requireV1Session(); err != nil {
		return "", err
	}
	return a.glpi.GetActiveProfileName(a.ctx, a.session)
}

// FindComputerBySerial looks up a GLPI Computer by serial number. Read-only.
func (a *App) FindComputerBySerial(serial string) ([]glpi.Computer, error) {
	if a.session == nil {
		return nil, errors.New("nicht angemeldet")
	}
	return a.glpi.FindBySerial(a.ctx, a.session, serial)
}

// DetectHardware inspects the machine the app is currently running on —
// the live counterpart to parsing a toolstar report from USB, for a device
// with no such report yet (see internal/hwinfo).
func (a *App) DetectHardware() (*hwinfo.Info, error) {
	return hwinfo.Detect()
}

// MemoryModuleInput and DiskInput carry the technician-edited values from
// the "Neues Gerät" form for one hardware component — see NewDeviceInput.
type MemoryModuleInput struct {
	CapacityGB float64 `json:"capacityGB"`
	DDRType    string  `json:"ddrType"`
	SpeedMHz   int     `json:"speedMHz"`
	FormFactor string  `json:"formFactor"`
}

type DiskInput struct {
	Kind       string  `json:"kind"`
	CapacityGB float64 `json:"capacityGB"`
}

// NewDeviceInput is everything the "Neues Gerät" screen collects — the
// hwinfo-detected values, as confirmed/edited by the technician — needed to
// take a device from no GLPI entry at all to fully entered. No network
// adapters here on purpose — confirmed live against real finished computers
// that this GLPI instance doesn't track them (see glpi.FullDeviceInput).
type NewDeviceInput struct {
	Name         string              `json:"name"`
	Serial       string              `json:"serial"`
	Type         string              `json:"type"`
	Manufacturer string              `json:"manufacturer"`
	Model        string              `json:"model"`
	OSName       string              `json:"osName"`
	OSVersion    string              `json:"osVersion"`
	CPUModel     string              `json:"cpuModel"`
	GPUModel     string              `json:"gpuModel"`
	Memory       []MemoryModuleInput `json:"memory"`
	Disks        []DiskInput         `json:"disks"`
}

// CreateDeviceResult is the outcome of CreateFullComputer. Computer is set
// whenever the call succeeds at all (a Go error is only returned if the
// Computer record itself couldn't be created). Warnings, if non-empty,
// lists components that couldn't be found/created/linked afterwards (see
// glpi.CreateFullComputer) — for the frontend to surface without blocking
// the technician from continuing to the review screen.
type CreateDeviceResult struct {
	Computer glpi.Computer `json:"computer"`
	Warnings []string      `json:"warnings,omitempty"`
}

// CreateFullComputer creates the Computer record in GLPI and attaches
// everything the technician confirmed on the "Neues Gerät" screen —
// manufacturer, model, type, operating system, and hardware components
// (CPU, RAM, disks, GPU, network adapters). This is what actually takes a
// device from no GLPI entry at all to fully entered: component catalog
// entries are found-or-created as needed (see internal/glpi). A write; not
// exercised live during development, per [[glpi_write_approval_rule]].
func (a *App) CreateFullComputer(input NewDeviceInput) (CreateDeviceResult, error) {
	if err := a.requireV1Session(); err != nil {
		return CreateDeviceResult{}, err
	}
	full := toFullDeviceInput(input)

	// The frontend already asks before creating anything, but only
	// administrator mode may add new entries to GLPI's shared dropdowns and
	// catalogs — enforce that here too rather than trusting the UI.
	if !a.GetAdministrator() {
		missing, err := a.glpi.FindMissing(a.ctx, a.session, full)
		if err != nil {
			return CreateDeviceResult{}, fmt.Errorf("prüfung in GLPI fehlgeschlagen: %w", err)
		}
		if len(missing) > 0 {
			return CreateDeviceResult{}, errors.New("nicht alle Einträge sind in GLPI vorhanden — nur im Administrator-Modus können sie angelegt werden")
		}
	}

	computer, warnings, err := a.glpi.CreateFullComputer(a.ctx, a.session, full, a.GetAdministrator())
	if err != nil {
		return CreateDeviceResult{}, fmt.Errorf("gerät konnte nicht angelegt werden: %w", err)
	}
	return CreateDeviceResult{Computer: *computer, Warnings: warnings}, nil
}

// UpdateFullComputer updates an existing Computer's identity fields (name,
// serial, manufacturer, model, type) — the "edit the existing device" path
// after a serial-number collision. Hardware components/OS are intentionally
// left untouched; see glpi.UpdateFullComputer.
func (a *App) UpdateFullComputer(computerID int, input NewDeviceInput) (CreateDeviceResult, error) {
	if err := a.requireV1Session(); err != nil {
		return CreateDeviceResult{}, err
	}
	full := toFullDeviceInput(input)

	if !a.GetAdministrator() {
		missing, err := a.glpi.FindMissing(a.ctx, a.session, full)
		if err != nil {
			return CreateDeviceResult{}, fmt.Errorf("prüfung in GLPI fehlgeschlagen: %w", err)
		}
		if len(missing) > 0 {
			return CreateDeviceResult{}, errors.New("nicht alle Einträge sind in GLPI vorhanden — nur im Administrator-Modus können sie angelegt werden")
		}
	}

	computer, warnings, err := a.glpi.UpdateFullComputer(a.ctx, a.session, computerID, full, a.GetAdministrator())
	if err != nil {
		return CreateDeviceResult{}, fmt.Errorf("gerät konnte nicht aktualisiert werden: %w", err)
	}
	return CreateDeviceResult{Computer: *computer, Warnings: warnings}, nil
}

// CheckNewDevice lists, without writing anything, every manufacturer/model/
// type/OS/component entry the device needs that GLPI doesn't have yet.
func (a *App) CheckNewDevice(input NewDeviceInput) ([]glpi.MissingEntry, error) {
	if err := a.requireV1Session(); err != nil {
		return nil, err
	}
	return a.glpi.FindMissing(a.ctx, a.session, toFullDeviceInput(input))
}

func (a *App) requireV1Session() error {
	if a.session == nil {
		return errors.New("nicht angemeldet")
	}
	if a.session.V1SessionToken == "" {
		return errors.New("keine v1-Sitzung — bitte Passwort erneut eingeben")
	}
	return nil
}

func toFullDeviceInput(input NewDeviceInput) glpi.FullDeviceInput {
	memory := make([]glpi.MemoryModuleInput, len(input.Memory))
	for i, m := range input.Memory {
		memory[i] = glpi.MemoryModuleInput{CapacityGB: m.CapacityGB, DDRType: m.DDRType, SpeedMHz: m.SpeedMHz, FormFactor: m.FormFactor}
	}
	disks := make([]glpi.DiskInput, len(input.Disks))
	for i, d := range input.Disks {
		disks[i] = glpi.DiskInput{Kind: d.Kind, CapacityGB: d.CapacityGB}
	}
	return glpi.FullDeviceInput{
		Name:         input.Name,
		Serial:       input.Serial,
		Type:         input.Type,
		Manufacturer: input.Manufacturer,
		Model:        input.Model,
		OSName:       input.OSName,
		OSVersion:    input.OSVersion,
		CPUModel:     input.CPUModel,
		GPUModel:     input.GPUModel,
		Memory:       memory,
		Disks:        disks,
	}
}

// EinsetzbarStatusID is the live GLPI status id confirmed for "finished"
// devices (see project memory — do not re-derive this from a filter query,
// it was confirmed against real data and there is no "Verkaufsbereit"
// status on this instance despite what other tools might assume).
const EinsetzbarStatusID = 2

type DocumentToUpload struct {
	Filename   string `json:"filename"`
	SourcePath string `json:"sourcePath"`
	// Content, when non-empty, is used directly instead of reading
	// SourcePath — for generated documents (the spec-sheet TXT) that
	// never existed as a file on disk.
	Content            []byte `json:"content,omitempty"`
	DocumentCategoryID int    `json:"documentCategoryID"`
}

// SpecSheetCategoryID is GLPI's confirmed Document Category for the
// finished-device spec sheet TXT ("TXT Vorlage Online Shop").
const SpecSheetCategoryID = 2

// GenerateSpecSheetTXT renders the finished-device spec sheet in the exact
// format GLPI already has real examples of (see project memory
// [[finished_device_txt_format]]). Pure text generation, no GLPI call.
func (a *App) GenerateSpecSheetTXT(fields specsheet.Fields) string {
	return specsheet.Generate(fields)
}

type FinishDeviceInput struct {
	ComputerID  int                    `json:"computerID"`
	Zusatzdaten glpi.ZusatzdatenUpdate `json:"zusatzdaten"`
	Documents   []DocumentToUpload     `json:"documents"`
}

// FinishDevice performs the actual write to GLPI: sets status to
// "Einsetzbar", updates the Zusatzdaten condition fields, and uploads each
// given document. This is the one place in the app that writes anything —
// see [[glpi_write_approval_rule]] in project memory: during development
// this was never invoked without the user's explicit per-request
// go-ahead, and it's built from documented conventions rather than a
// live-verified request shape (see internal/glpi/write.go).
func (a *App) FinishDevice(input FinishDeviceInput) error {
	if a.session == nil {
		return errors.New("nicht angemeldet")
	}
	if a.session.V1SessionToken == "" {
		return errors.New("keine v1-Sitzung — bitte Passwort erneut eingeben")
	}

	if err := a.glpi.SetComputerStatus(a.ctx, a.session, input.ComputerID, EinsetzbarStatusID); err != nil {
		return fmt.Errorf("status konnte nicht gesetzt werden: %w", err)
	}
	if err := a.glpi.UpdateZusatzdaten(a.ctx, a.session, input.ComputerID, input.Zusatzdaten); err != nil {
		return fmt.Errorf("Zusatzdaten konnten nicht gespeichert werden: %w", err)
	}
	for _, doc := range input.Documents {
		content := doc.Content
		if len(content) == 0 {
			var err error
			content, err = os.ReadFile(doc.SourcePath)
			if err != nil {
				return fmt.Errorf("Dokument %q konnte nicht gelesen werden: %w", doc.Filename, err)
			}
		}
		if err := a.glpi.UploadDocument(a.ctx, a.session, input.ComputerID, doc.Filename, content, doc.DocumentCategoryID); err != nil {
			return fmt.Errorf("Dokument %q konnte nicht hochgeladen werden: %w", doc.Filename, err)
		}
	}
	return nil
}

func humanizeLoginError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "credentials were incorrect") {
		return "Benutzername oder Passwort ist falsch."
	}
	return fmt.Sprintf("Login fehlgeschlagen: %s", msg)
}
