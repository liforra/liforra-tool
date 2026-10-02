// Component linking: attaching catalog entries to a specific Computer via
// GLPI's Item_Device* link tables, plus the operating system link
// (Item_OperatingSystem), and CreateFullComputer — the orchestration that
// takes a technician's "Neues Gerät" scan all the way from no GLPI entry at
// all to a fully entered Computer. Existing entries are found with the
// tolerant matcher in match.go.
//
// Confirmed live on 2026-09-18, end to end, against a real throwaway device
// (Computer id 910, "LiforraTestDevice", since deleted — created with the
// user's explicit per-request go-ahead, see [[glpi_write_approval_rule]] in
// project memory): CreateComputer via v1 with manufacturers_id/
// computertypes_id/computermodels_id, read back correctly via v2; OS linked
// via Item_OperatingSystem; Processor/GraphicCard/Memory/HardDrive each
// linked via the corresponding Item_Device* table — every field name below
// matched what the v2 Component/* read-back showed.
package glpi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
)

// linkItem POSTs a v1 generic-itemtype create for one of GLPI's Item_Device*
// (or Item_OperatingSystem) link tables.
func (c *Client) linkItem(ctx context.Context, sess *Session, linkItemtype string, fields map[string]any) error {
	body, err := json.Marshal(map[string]any{"input": fields})
	if err != nil {
		return err
	}
	reqURL := fmt.Sprintf("%s/api.php/v1/%s", c.BaseURL, linkItemtype)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("App-Token", c.V1AppToken)
	req.Header.Set("Session-Token", sess.V1SessionToken)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// MemoryModuleInput / DiskInput mirror the relevant hwinfo fields,
// deliberately decoupled from that package — see main.NewDeviceInput in
// app.go for the wails-facing type these get mapped from.
type MemoryModuleInput struct {
	CapacityGB float64
	DDRType    string
	SpeedMHz   int
	// FormFactor is "DIMM" or "SO-DIMM" as reported by the hardware; empty
	// falls back to guessing from the device type.
	FormFactor string
}

// DiskInput carries no model or serial: confirmed live (2026-09-18) that
// only 3 of 741 disk links in this GLPI have a serial and none are
// identified by model — a disk is just its catalog entry.
type DiskInput struct {
	// Kind is the catalog prefix, e.g. "M.2 SATA SSD" — detected by hwinfo,
	// correctable by the technician.
	Kind       string
	CapacityGB float64
}

// FullDeviceInput is everything needed to take a device from no GLPI entry
// at all to fully entered: the Computer record itself plus every hardware
// component the technician confirmed on the "Neues Gerät" screen.
//
// Network adapters are deliberately not part of this — confirmed live
// (2026-09-18) against several real finished ("Einsetzbar") computers that
// this GLPI instance never populates network components on them (v2
// Computer.network is also always null, and Component/NetworkCard is
// always empty), so there's nothing to write or check for them either.
type FullDeviceInput struct {
	Name         string
	Serial       string
	Type         string
	Manufacturer string
	Model        string
	OSName       string
	OSVersion    string
	CPUModel     string
	GPUModel     string
	Memory       []MemoryModuleInput
	Disks        []DiskInput
}

// MissingEntry is one dropdown or catalog entry CreateFullComputer would
// have to create because GLPI has no match for it yet. Key names the form
// field it came from ("cpu", "disk-0", ...); Kind is the label shown to the
// technician.
type MissingEntry struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type plannedLookup struct {
	key, kind, itemtype string
	// field is "name" for dropdowns, "designation" for Device* catalogs.
	field string
	name  string
	hints []string
}

// plannedLookups lists every lookup CreateFullComputer performs for input.
// FindMissing runs the same list, so what the technician is warned about
// and what actually gets created can't drift apart.
func plannedLookups(input FullDeviceInput) []plannedLookup {
	l := []plannedLookup{
		{key: "manufacturer", kind: "Hersteller", itemtype: "Manufacturer", field: "name", name: input.Manufacturer},
		{key: "model", kind: "Modell", itemtype: "ComputerModel", field: "name", name: input.Model, hints: []string{input.Manufacturer}},
		{key: "type", kind: "Typ", itemtype: "ComputerType", field: "name", name: input.Type},
		{key: "os", kind: "Betriebssystem", itemtype: "OperatingSystem", field: "name", name: input.OSName},
		{key: "cpu", kind: "Prozessor", itemtype: "DeviceProcessor", field: "designation", name: input.CPUModel},
		{key: "gpu", kind: "Grafikkarte", itemtype: "DeviceGraphicCard", field: "designation", name: input.GPUModel},
	}
	if input.OSName != "" {
		l = append(l, plannedLookup{key: "osVersion", kind: "Betriebssystem-Version", itemtype: "OperatingSystemVersion", field: "name", name: input.OSVersion})
	}
	isLaptop := isLaptopType(input.Type)
	for i, m := range input.Memory {
		l = append(l, plannedLookup{key: fmt.Sprintf("memory-%d", i), kind: "Arbeitsspeicher", itemtype: "DeviceMemory", field: "designation", name: memoryDesignation(m, isLaptop)})
	}
	for i, d := range input.Disks {
		l = append(l, plannedLookup{key: fmt.Sprintf("disk-%d", i), kind: "Laufwerk", itemtype: "DeviceHardDrive", field: "designation", name: diskDesignation(d)})
	}
	return l
}

// FindMissing reports, without writing anything, which entries
// CreateFullComputer would have to create for input.
func (c *Client) FindMissing(ctx context.Context, sess *Session, input FullDeviceInput) ([]MissingEntry, error) {
	var missing []MissingEntry
	for _, p := range plannedLookups(input) {
		if p.name == "" {
			continue
		}
		id, err := c.findBestMatch(ctx, sess, p.itemtype, p.field, p.name, p.hints)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.kind, err)
		}
		if id == 0 {
			missing = append(missing, MissingEntry{Key: p.key, Kind: p.kind, Name: p.name})
		}
	}
	return missing, nil
}

// resolveOrCreate looks up p in GLPI's catalog and, only when allowCreate is
// true, creates it if nothing matched. allowCreate must reflect this app's
// own "Administrator" setting (see app.go's GetAdministrator) — enforced
// *here*, not just as an earlier, separate pre-flight check (FindMissing)
// that a caller might skip or whose result might have gone stale by the
// time this runs. Without this, a non-admin technician's create attempt
// would reach GLPI's own create-dropdown-entry call, which almost always
// 403s for an account that only has asset-write (not catalog-admin)
// rights — a real GLPI permissions error, but one this app should refuse
// to even attempt, with its own clear message, rather than let happen.
func (c *Client) resolveOrCreate(ctx context.Context, sess *Session, p plannedLookup, allowCreate bool) (int, error) {
	if p.name == "" {
		return 0, nil
	}
	id, err := c.findBestMatch(ctx, sess, p.itemtype, p.field, p.name, p.hints)
	if err != nil || id != 0 {
		return id, err
	}
	if !allowCreate {
		return 0, fmt.Errorf("%q ist nicht in GLPI vorhanden — nur im Administrator-Modus kann es angelegt werden", p.name)
	}
	return c.createEntry(ctx, sess, p.itemtype, p.field, p.name)
}

// CreateFullComputer creates the Computer record and attaches everything in
// input to it: resolves/creates the Manufacturer, ComputerModel and
// ComputerType dropdowns before creating the Computer (so it's created with
// the right references from the start), then resolves/creates and links
// the operating system and each hardware component.
//
// Only a failure to create the Computer record itself (or to resolve the
// three dropdowns needed for it) is a hard error. Once the Computer exists,
// each component is best-effort: a failure there is collected as a warning
// and returned alongside the created Computer rather than aborting, since a
// technician would rather have a Computer with 4 out of 5 components
// entered — and fix the rest directly in GLPI — than nothing at all.
func (c *Client) CreateFullComputer(ctx context.Context, sess *Session, input FullDeviceInput, admin bool) (*Computer, []string, error) {
	lookups := map[string]plannedLookup{}
	for _, p := range plannedLookups(input) {
		lookups[p.key] = p
	}

	ids := map[string]int{}
	for _, key := range []string{"manufacturer", "model", "type"} {
		id, err := c.resolveOrCreate(ctx, sess, lookups[key], admin)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", lookups[key].kind, err)
		}
		ids[key] = id
	}

	computer, err := c.CreateComputer(ctx, sess, NewComputerInput{
		Name:             input.Name,
		Serial:           input.Serial,
		ManufacturersID:  ids["manufacturer"],
		ComputerModelsID: ids["model"],
		ComputerTypesID:  ids["type"],
	})
	if err != nil {
		return nil, nil, err
	}

	var warnings []string
	attempt := func(p plannedLookup, fn func() error) {
		if err := fn(); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s „%s“: %v", p.kind, p.name, err))
		}
	}

	if osLookup := lookups["os"]; osLookup.name != "" {
		attempt(osLookup, func() error {
			return c.linkOperatingSystem(ctx, sess, computer.ID, osLookup, lookups["osVersion"], admin)
		})
	}

	type component struct{ key, linkItemtype, foreignKey string }
	components := []component{
		{"cpu", "Item_DeviceProcessor", "deviceprocessors_id"},
		{"gpu", "Item_DeviceGraphicCard", "devicegraphiccards_id"},
	}
	for i := range input.Memory {
		components = append(components, component{fmt.Sprintf("memory-%d", i), "Item_DeviceMemory", "devicememories_id"})
	}
	for i := range input.Disks {
		components = append(components, component{fmt.Sprintf("disk-%d", i), "Item_DeviceHardDrive", "deviceharddrives_id"})
	}
	for _, comp := range components {
		p := lookups[comp.key]
		if p.name == "" {
			continue
		}
		attempt(p, func() error {
			return c.linkComponent(ctx, sess, computer.ID, p, comp.linkItemtype, comp.foreignKey, admin)
		})
	}

	return computer, warnings, nil
}

// UpdateFullComputer updates an existing Computer's identity fields (name,
// serial, manufacturer, model, type) when a technician chooses "edit the
// existing device" after a serial-number collision — GLPI's data is
// authoritative there, so this always writes whatever the (GLPI-prefilled,
// technician-editable) form currently holds.
//
// Deliberately does NOT touch OS or hardware components (CPU/RAM/disks/GPU):
// unlike CreateFullComputer, which links components onto a record it just
// created and knows to be empty, an existing device may already have real
// component links in GLPI, and there's no live-verified way yet to read
// those back and reconcile them without risking duplicate or destructive
// writes (see internal/glpi package comment on the create path's own
// live-verification history). Re-linking is intentionally left as a
// follow-up once that read-back path is confirmed live, not silently
// guessed at here.
func (c *Client) UpdateFullComputer(ctx context.Context, sess *Session, computerID int, input FullDeviceInput, admin bool) (*Computer, []string, error) {
	lookups := map[string]plannedLookup{}
	for _, p := range plannedLookups(input) {
		lookups[p.key] = p
	}

	ids := map[string]int{}
	for _, key := range []string{"manufacturer", "model", "type"} {
		id, err := c.resolveOrCreate(ctx, sess, lookups[key], admin)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", lookups[key].kind, err)
		}
		ids[key] = id
	}

	computer, err := c.UpdateComputer(ctx, sess, computerID, NewComputerInput{
		Name:             input.Name,
		Serial:           input.Serial,
		ManufacturersID:  ids["manufacturer"],
		ComputerModelsID: ids["model"],
		ComputerTypesID:  ids["type"],
	})
	if err != nil {
		return nil, nil, err
	}

	warnings := []string{"Hardware-Komponenten (Betriebssystem/CPU/RAM/Laufwerke/Grafik) wurden nicht verändert — vorhandene Einträge in GLPI bleiben wie sie sind."}
	return computer, warnings, nil
}

func (c *Client) linkComponent(ctx context.Context, sess *Session, computerID int, p plannedLookup, linkItemtype, foreignKey string, admin bool) error {
	id, err := c.resolveOrCreate(ctx, sess, p, admin)
	if err != nil || id == 0 {
		return err
	}
	return c.linkItem(ctx, sess, linkItemtype, map[string]any{"items_id": computerID, "itemtype": "Computer", foreignKey: id})
}

// linkOperatingSystem resolves/creates the OperatingSystem (and, if given,
// OperatingSystemVersion) and links both to the Computer via
// Item_OperatingSystem — GLPI keeps OS data on that link table, not on the
// Computer record itself.
func (c *Client) linkOperatingSystem(ctx context.Context, sess *Session, computerID int, osLookup, version plannedLookup, admin bool) error {
	osID, err := c.resolveOrCreate(ctx, sess, osLookup, admin)
	if err != nil || osID == 0 {
		return err
	}
	fields := map[string]any{"items_id": computerID, "itemtype": "Computer", "operatingsystems_id": osID}
	versionID, err := c.resolveOrCreate(ctx, sess, version, admin)
	if err != nil {
		return err
	}
	if versionID != 0 {
		fields["operatingsystemversions_id"] = versionID
	}
	return c.linkItem(ctx, sess, "Item_OperatingSystem", fields)
}

func isLaptopType(t string) bool {
	t = strings.ToLower(t)
	for _, w := range []string{"laptop", "notebook", "ultrabook", "convertible", "tablet"} {
		if strings.Contains(t, w) {
			return true
		}
	}
	return false
}

// memoryDesignation builds a catalog designation in this GLPI's convention,
// e.g. "SO-DIMM DDR4 4GB 2133MHz" — a generic bucket, not a per-stick model
// name; the per-link "size" field stays 0 in every real example.
func memoryDesignation(m MemoryModuleInput, isLaptop bool) string {
	formFactor := m.FormFactor
	if formFactor == "" {
		formFactor = "DIMM"
		if isLaptop {
			formFactor = "SO-DIMM"
		}
	}
	parts := []string{formFactor}
	if m.DDRType != "" {
		parts = append(parts, m.DDRType)
	}
	if m.CapacityGB > 0 {
		parts = append(parts, fmt.Sprintf("%dGB", int(math.Round(m.CapacityGB))))
	}
	if m.SpeedMHz > 0 {
		parts = append(parts, fmt.Sprintf("%dMHz", m.SpeedMHz))
	}
	return strings.Join(parts, " ")
}

// diskDesignation builds a catalog designation in this GLPI's convention:
// the kind ("SATA SSD", "M.2 SATA SSD", "mSATA SSD", "M.2 NVME SSD",
// "SATA HDD") plus capacity, in TB from 1000GB up — e.g. "M.2 SATA SSD
// 256GB", "SATA HDD 1TB".
func diskDesignation(d DiskInput) string {
	var parts []string
	if d.Kind != "" {
		parts = append(parts, d.Kind)
	}
	switch {
	case d.CapacityGB >= 1000:
		parts = append(parts, fmt.Sprintf("%dTB", int(math.Round(d.CapacityGB/1000))))
	case d.CapacityGB > 0:
		parts = append(parts, fmt.Sprintf("%dGB", int(math.Round(d.CapacityGB))))
	}
	return strings.Join(parts, " ")
}
