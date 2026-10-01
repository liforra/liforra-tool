// Package glpi is a minimal client for the parts of GLPI's REST APIs (v1 and
// the newer v2 "high-level" API) that this tool needs. Auth uses OAuth2's
// password grant (v2) alongside a classic v1 session, obtained from a single
// technician login — see project memory for why (GLPI's authorization_code
// grant is broken on this instance due to a server-side timezone bug outside
// our control).
package glpi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Session holds everything needed to make authenticated requests after login.
// Never persisted to disk by this package — callers decide if/how to cache it.
type Session struct {
	AccessToken    string
	RefreshToken   string
	ExpiresAt      time.Time
	V1SessionToken string
	Username       string
	// UserID is 0 until GetCurrentUserID has been called once — it's only
	// needed for setting "Verantwortlicher Techniker" to the logged-in
	// user, not for anything on the critical login path, so it's fetched
	// lazily and cached here rather than during Login/Refresh.
	UserID int
}

// Client talks to one GLPI instance.
type Client struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	V1AppToken   string
	HTTP         *http.Client
}

func NewClient(baseURL, clientID, clientSecret, v1AppToken string) *Client {
	return &Client{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		V1AppToken:   v1AppToken,
		HTTP:         &http.Client{Timeout: 30 * time.Second},
	}
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// Login performs both the OAuth2 password-grant login (v2 API) and a classic
// v1 session init, using the same credentials for both. The password is used
// only in-memory for these two requests and never stored or logged.
func (c *Client) Login(ctx context.Context, username, password string) (*Session, error) {
	sess := &Session{Username: username}

	tok, err := c.passwordGrant(ctx, username, password)
	if err != nil {
		return nil, fmt.Errorf("v2 login failed: %w", err)
	}
	sess.AccessToken = tok.AccessToken
	sess.RefreshToken = tok.RefreshToken
	sess.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)

	v1tok, err := c.initV1Session(ctx, username, password)
	if err != nil {
		return nil, fmt.Errorf("v1 session init failed: %w", err)
	}
	sess.V1SessionToken = v1tok

	return sess, nil
}

// Refresh gets a new access token using the stored refresh token, without
// requiring the user to log in again.
func (c *Client) Refresh(ctx context.Context, sess *Session) error {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {sess.RefreshToken},
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
	}
	tok, err := c.tokenRequest(ctx, form)
	if err != nil {
		return fmt.Errorf("refresh failed: %w", err)
	}
	sess.AccessToken = tok.AccessToken
	if tok.RefreshToken != "" {
		sess.RefreshToken = tok.RefreshToken
	}
	sess.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return nil
}

func (c *Client) passwordGrant(ctx context.Context, username, password string) (*oauthTokenResponse, error) {
	form := url.Values{
		"grant_type":    {"password"},
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"username":      {username},
		"password":      {password},
		"scope":         {"api"},
	}
	return c.tokenRequest(ctx, form)
}

func (c *Client) tokenRequest(ctx context.Context, form url.Values) (*oauthTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api.php/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tok oauthTokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("unexpected response (status %d): %s", resp.StatusCode, string(body))
	}
	if resp.StatusCode != http.StatusOK {
		if tok.ErrorDesc != "" {
			return nil, fmt.Errorf("%s: %s", tok.Error, tok.ErrorDesc)
		}
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return &tok, nil
}

type v1InitSessionResponse struct {
	SessionToken string `json:"session_token"`
}

// InitV1Session opens a classic v1 API session on its own, without also
// doing the v2 OAuth login — used to silently restore v1 access from a
// persisted password (see internal/sessionstore) without bothering the
// technician for a fresh login.
func (c *Client) InitV1Session(ctx context.Context, username, password string) (string, error) {
	return c.initV1Session(ctx, username, password)
}

// v1FullSessionResponse is getFullSession's shape as GLPI's own apirest.md
// documents it. ActiveProfile in particular is NOT yet confirmed against a
// real response (unlike most of this package's GLPI field assumptions) —
// treat the field name as a strong first draft, same caveat as write.go,
// until checked live against an account with more than one profile.
type v1FullSessionResponse struct {
	Session struct {
		GLPIID        int `json:"glpiID"`
		ActiveProfile struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"glpiactiveprofile"`
	} `json:"session"`
}

func (c *Client) fetchFullSession(ctx context.Context, sess *Session) (*v1FullSessionResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api.php/v1/getFullSession", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("App-Token", c.V1AppToken)
	req.Header.Set("Session-Token", sess.V1SessionToken)

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

	var full v1FullSessionResponse
	if err := json.Unmarshal(body, &full); err != nil {
		return nil, fmt.Errorf("unexpected response: %s", string(body))
	}
	return &full, nil
}

// GetCurrentUserID returns the numeric GLPI user id behind the current v1
// session, via GLPI's standard getFullSession endpoint — used to set
// "Verantwortlicher Techniker" to "whoever is logged in right now" without
// the technician looking up their own id. Cache the result on
// Session.UserID rather than calling this every time.
func (c *Client) GetCurrentUserID(ctx context.Context, sess *Session) (int, error) {
	full, err := c.fetchFullSession(ctx, sess)
	if err != nil {
		return 0, err
	}
	if full.Session.GLPIID == 0 {
		return 0, fmt.Errorf("no glpiID in session response")
	}
	return full.Session.GLPIID, nil
}

// GetActiveProfileName returns the name of the GLPI profile currently
// active for this session (e.g. "Self-Service", "Technician",
// "Super-Admin") — surfaced in the app so a technician who can create
// devices on the GLPI website but not here has an immediate, concrete thing
// to check: GLPI logs a multi-profile account into whichever profile is
// its default, which may not be the one with asset-write rights, and
// nothing about that is visible from a plain login. This app never
// switches profiles on its own (see GLPI's changeActiveProfile endpoint for
// that, not implemented here) — it only reports what's currently active.
func (c *Client) GetActiveProfileName(ctx context.Context, sess *Session) (string, error) {
	full, err := c.fetchFullSession(ctx, sess)
	if err != nil {
		return "", err
	}
	if full.Session.ActiveProfile.Name == "" {
		return "", fmt.Errorf("no active profile in session response")
	}
	return full.Session.ActiveProfile.Name, nil
}

func (c *Client) initV1Session(ctx context.Context, username, password string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api.php/v1/initSession", nil)
	if err != nil {
		return "", err
	}
	basic := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	req.Header.Set("Authorization", "Basic "+basic)
	req.Header.Set("App-Token", c.V1AppToken)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var v1resp v1InitSessionResponse
	if err := json.Unmarshal(body, &v1resp); err != nil {
		return "", fmt.Errorf("unexpected response: %s", string(body))
	}
	return v1resp.SessionToken, nil
}
