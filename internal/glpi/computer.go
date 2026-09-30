package glpi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type NamedRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Computer struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	Serial       string   `json:"serial"`
	OtherSerial  string   `json:"otherserial"`
	Comment      string   `json:"comment"`
	Status       NamedRef `json:"status"`
	Location     NamedRef `json:"location"`
	Type         NamedRef `json:"type"`
	Manufacturer NamedRef `json:"manufacturer"`
	Model        NamedRef `json:"model"`
}

// FindBySerial searches for a Computer by its serial number (v2 API, read-only).
func (c *Client) FindBySerial(ctx context.Context, sess *Session, serial string) ([]Computer, error) {
	filter := fmt.Sprintf("serial==%s", serial)
	q := url.Values{"filter": {filter}, "limit": {"10"}}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.BaseURL+"/api.php/v2.1/Assets/Computer?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+sess.AccessToken)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var computers []Computer
	if err := json.Unmarshal(body, &computers); err != nil {
		return nil, fmt.Errorf("unexpected response: %s", string(body))
	}
	return computers, nil
}

// GetComputer fetches a single Computer by id (v2 API, read-only) — used
// after a v1 create/update to read back the full record with its
// manufacturer/model/type/status references resolved to names, since v1's
// generic create only returns an id (see write.go's CreateComputer).
func (c *Client) GetComputer(ctx context.Context, sess *Session, id int) (*Computer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api.php/v2.1/Assets/Computer/%d", c.BaseURL, id), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+sess.AccessToken)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var computer Computer
	if err := json.Unmarshal(body, &computer); err != nil {
		return nil, fmt.Errorf("unexpected response: %s", string(body))
	}
	return &computer, nil
}
