// Write-side GLPI calls: creating/updating a Computer, setting its
// "Computer-Daten | Zusatz" condition fields, and attaching documents.
//
// IMPORTANT: none of this has been exercised against the live instance —
// per project rules, no write request runs without the user's explicit
// per-request go-ahead, so this is built from documented/observed GLPI REST
// conventions (see project memory) rather than confirmed live. Treat the
// request shapes here as a strong first draft, not verified fact, and test
// carefully (ideally against a throwaway/test device) before relying on it.
package glpi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// ZusatzdatenUpdate is the "Computer-Daten | Zusatz" condition-assessment
// fields — see project memory for why only these (not the full physical
// spec sheet) are written to GLPI's custom fields. Pointer fields are only
// sent if non-nil, so a partial update doesn't clobber fields the caller
// didn't set.
type ZusatzdatenUpdate struct {
	Maengel *string `json:"mngelfield,omitempty"`
	// AkkugesundheitPct/Akkugesundheit2Pct are strings, not numbers —
	// confirmed live 2026-09-29 against real Computer records
	// ("akkugesundheitinfield":"15.9", "90.7", "81", ...): this is a
	// free-text plugin field in GLPI's schema, not an integer column.
	// Sending a JSON number here was silently rejected/ignored — this was
	// this feature's whole bug.
	AkkugesundheitPct       *string `json:"akkugesundheitinfield,omitempty"`
	Akkugesundheit2Pct      *string `json:"akkugesundheittwoinfield,omitempty"`
	BetriebssystemAktiviert *bool   `json:"betriebssystemaktiviertfield,omitempty"`
	// Funktionsfaehigkeit/Extras are dropdown option IDs, not resolved to
	// labels yet (see project memory) — left as raw IDs for now.
	FunktionsfaehigkeitIDs []int `json:"-"`
	ExtrasIDs              []int `json:"-"`
}

// UpdateZusatzdaten PATCHes the Computer's "Zusatz-Daten" plugin-fields
// record (v1 API — this data isn't reachable via v2, see project memory).
func (c *Client) UpdateZusatzdaten(ctx context.Context, sess *Session, computerID int, update ZusatzdatenUpdate) error {
	body, err := json.Marshal(map[string]any{"input": update})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api.php/v1/Computer/%d/PluginFieldsComputerzusatzdaten", c.BaseURL, computerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
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

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// v1CreateResponse is the common shape of a v1 generic-itemtype create
// response ({"id": 123, "message": "..."}) — shared by every write in this
// package that creates a record via that convention (catalog entries,
// dropdowns, component links).
type v1CreateResponse struct {
	ID int `json:"id"`
}

// NewComputerInput creates a Computer record. Manufacturer/model/type are
// GLPI dropdown references (not free text) — see match.go for how a
// technician's free-text entry gets resolved to one of these ids. Zero
// means "leave unset" for each.
type NewComputerInput struct {
	Name             string `json:"name"`
	Serial           string `json:"serial"`
	ManufacturersID  int    `json:"manufacturers_id,omitempty"`
	ComputerModelsID int    `json:"computermodels_id,omitempty"`
	ComputerTypesID  int    `json:"computertypes_id,omitempty"`
}

// CreateComputer creates a new Computer record via the v1 API and returns
// the created record (re-fetched via v2, see GetComputer). v1, not v2, on
// purpose: confirmed live (2026-09-18, via GLPI's own v2.1 OpenAPI spec —
// api.php/v2.1.0/doc.json) that the v2 Computer schema marks "type" and
// "model" as readOnly, so a v2 POST silently can't set them — v1's classic
// per-field write does. The whole thing (manufacturers_id/computertypes_id/
// computermodels_id via v1 create, read back correctly via v2) was
// end-to-end verified live the same day against a real throwaway device
// (Computer id 910, "LiforraTestDevice") — see components.go's package
// comment.
func (c *Client) CreateComputer(ctx context.Context, sess *Session, input NewComputerInput) (*Computer, error) {
	body, err := json.Marshal(map[string]any{"input": input})
	if err != nil {
		return nil, err
	}
	url := c.BaseURL + "/api.php/v1/Computer"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("App-Token", c.V1AppToken)
	req.Header.Set("Session-Token", sess.V1SessionToken)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	var created v1CreateResponse
	if err := json.Unmarshal(respBody, &created); err != nil {
		return nil, fmt.Errorf("unexpected response: %s", string(respBody))
	}
	return c.GetComputer(ctx, sess, created.ID)
}

// UpdateComputer PUTs the Computer record's own top-level fields (v1, same
// per-field write already confirmed live for CreateComputer/
// SetResponsibleFields — v2 marks type/model readOnly). Only used for the
// "edit an existing device" flow's identity fields (name/serial/
// manufacturer/model/type); it deliberately does not touch OS or hardware
// components — see UpdateFullComputer in components.go for why.
func (c *Client) UpdateComputer(ctx context.Context, sess *Session, computerID int, input NewComputerInput) (*Computer, error) {
	fields := map[string]any{"id": computerID, "name": input.Name, "serial": input.Serial}
	if input.ManufacturersID != 0 {
		fields["manufacturers_id"] = input.ManufacturersID
	}
	if input.ComputerModelsID != 0 {
		fields["computermodels_id"] = input.ComputerModelsID
	}
	if input.ComputerTypesID != 0 {
		fields["computertypes_id"] = input.ComputerTypesID
	}
	body, err := json.Marshal(map[string]any{"input": fields})
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/api.php/v1/Computer/%d", c.BaseURL, computerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("App-Token", c.V1AppToken)
	req.Header.Set("Session-Token", sess.V1SessionToken)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return c.GetComputer(ctx, sess, computerID)
}

// SetComputerStatus updates just the status (e.g. to "Einsetzbar") via the
// v2 API.
func (c *Client) SetComputerStatus(ctx context.Context, sess *Session, computerID, statusID int) error {
	body, err := json.Marshal(map[string]any{"status": statusID})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api.php/v2.1/Assets/Computer/%d", c.BaseURL, computerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sess.AccessToken)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// SetResponsibleFields sets "Verantwortlicher Techniker" (users_id_tech),
// "Verantwortliche Gruppe" (groups_id_tech) and "Standort" (locations_id)
// in one PUT — the device-check page's "Fertig geprüft" button. Zero
// means "leave this one alone" for each (so callers that only resolved
// some of the three don't clobber the others). v1, not v2, same reasoning
// as CreateComputer: v2's Computer schema marks more fields readOnly than
// it advertises, and v1's classic per-field write is the one already
// confirmed to work for this kind of direct reference.
func (c *Client) SetResponsibleFields(ctx context.Context, sess *Session, computerID, userID, groupID, locationID int) error {
	input := map[string]any{"id": computerID}
	if userID != 0 {
		input["users_id_tech"] = userID
	}
	if groupID != 0 {
		input["groups_id_tech"] = groupID
	}
	if locationID != 0 {
		input["locations_id"] = locationID
	}
	body, err := json.Marshal(map[string]any{"input": input})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api.php/v1/Computer/%d", c.BaseURL, computerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
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

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// UploadDocument attaches a file to a Computer, following the classic GLPI
// REST API's multipart convention: a JSON "uploadManifest" part describing
// the target item plus a file part it references by name.
func (c *Client) UploadDocument(ctx context.Context, sess *Session, computerID int, filename string, content []byte, documentCategoryID int) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	manifest := map[string]any{
		"input": map[string]any{
			"name":      filename,
			"_filename": []string{filename},
			"items_id":  computerID,
			"itemtype":  "Computer",
		},
	}
	if documentCategoryID != 0 {
		manifest["input"].(map[string]any)["documentcategories_id"] = documentCategoryID
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := w.WriteField("uploadManifest", string(manifestJSON)); err != nil {
		return err
	}

	part, err := w.CreateFormFile("filename[0]", filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(content); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	url := c.BaseURL + "/api.php/v1/Document"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
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
