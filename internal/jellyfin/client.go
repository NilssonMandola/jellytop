// Package jellyfin is a small, purpose-built client for the subset of the
// Jellyfin server API that jellytop needs. It deliberately models only the
// fields the TUI renders rather than wrapping the full OpenAPI surface.
package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Client talks to a Jellyfin server using an API key.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New returns a Client for the server at baseURL authenticating with token.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// APIError is returned for any non-2xx response.
type APIError struct {
	Status int
	Method string
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized:
		return "unauthorized — check JELLYFIN_TOKEN (Dashboard → API Keys)"
	case http.StatusForbidden:
		return "forbidden — this API key lacks administrator rights"
	}
	msg := fmt.Sprintf("%s %s: HTTP %d", e.Method, e.Path, e.Status)
	if e.Body != "" {
		msg += ": " + truncate(e.Body, 200)
	}
	return msg
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		rdr = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", `MediaBrowser Token="`+c.token+`"`)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &APIError{Status: resp.StatusCode, Method: method, Path: path, Body: strings.TrimSpace(string(b))}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Ping verifies the server is reachable and the token is accepted.
func (c *Client) Ping(ctx context.Context) (*SystemInfo, error) {
	var info SystemInfo
	if err := c.do(ctx, http.MethodGet, "/System/Info", nil, nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// Sessions lists client sessions, playing ones first, then most recently active.
func (c *Client) Sessions(ctx context.Context) ([]Session, error) {
	var s []Session
	if err := c.do(ctx, http.MethodGet, "/Sessions", nil, nil, &s); err != nil {
		return nil, err
	}
	sort.SliceStable(s, func(i, j int) bool {
		if s[i].Playing() != s[j].Playing() {
			return s[i].Playing()
		}
		return s[i].LastActivityDate.After(s[j].LastActivityDate)
	})
	return s, nil
}

// StopPlayback tells a session to stop whatever it is playing.
func (c *Client) StopPlayback(ctx context.Context, sessionID string) error {
	return c.do(ctx, http.MethodPost, "/Sessions/"+url.PathEscape(sessionID)+"/Playing/Stop", nil, nil, nil)
}

// SendMessage displays a message on a session's client.
func (c *Client) SendMessage(ctx context.Context, sessionID, header, text string) error {
	body := map[string]any{"Header": header, "Text": text, "TimeoutMs": 10000}
	return c.do(ctx, http.MethodPost, "/Sessions/"+url.PathEscape(sessionID)+"/Message", nil, body, nil)
}

// Activity returns one page of the server activity log, newest first.
func (c *Client) Activity(ctx context.Context, startIndex, limit int) ([]ActivityEntry, int, error) {
	q := url.Values{
		"startIndex": {strconv.Itoa(startIndex)},
		"limit":      {strconv.Itoa(limit)},
	}
	var res activityResult
	if err := c.do(ctx, http.MethodGet, "/System/ActivityLog/Entries", q, nil, &res); err != nil {
		return nil, 0, err
	}
	return res.Items, res.TotalRecordCount, nil
}

// Users lists all users, sorted by name.
func (c *Client) Users(ctx context.Context) ([]User, error) {
	var u []User
	if err := c.do(ctx, http.MethodGet, "/Users", nil, nil, &u); err != nil {
		return nil, err
	}
	sort.Slice(u, func(i, j int) bool {
		return strings.ToLower(u[i].Name) < strings.ToLower(u[j].Name)
	})
	return u, nil
}

// setPolicyField reads the user's current policy, flips one boolean, and writes
// it back. Jellyfin's policy endpoint replaces the whole object, so the full
// document must be round-tripped or unmodelled settings would be reset.
func (c *Client) setPolicyField(ctx context.Context, userID, field string, value bool) error {
	var raw struct {
		Policy map[string]any `json:"Policy"`
	}
	if err := c.do(ctx, http.MethodGet, "/Users/"+url.PathEscape(userID), nil, nil, &raw); err != nil {
		return err
	}
	if raw.Policy == nil {
		return fmt.Errorf("user %s returned no policy", userID)
	}
	raw.Policy[field] = value
	return c.do(ctx, http.MethodPost, "/Users/"+url.PathEscape(userID)+"/Policy", nil, raw.Policy, nil)
}

// SetUserDisabled enables or disables a user account.
func (c *Client) SetUserDisabled(ctx context.Context, userID string, disabled bool) error {
	return c.setPolicyField(ctx, userID, "IsDisabled", disabled)
}

// SetUserAdmin grants or revokes administrator rights.
func (c *Client) SetUserAdmin(ctx context.Context, userID string, admin bool) error {
	return c.setPolicyField(ctx, userID, "IsAdministrator", admin)
}

// Libraries lists the configured media libraries.
func (c *Client) Libraries(ctx context.Context) ([]Library, error) {
	var l []Library
	if err := c.do(ctx, http.MethodGet, "/Library/VirtualFolders", nil, nil, &l); err != nil {
		return nil, err
	}
	return l, nil
}

// RefreshLibraries triggers a scan of all libraries. Jellyfin has no
// per-library scan endpoint; the whole-server scan is the only option.
func (c *Client) RefreshLibraries(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/Library/Refresh", nil, nil, nil)
}

// Tasks lists scheduled tasks, hidden ones excluded, grouped by category.
func (c *Client) Tasks(ctx context.Context) ([]Task, error) {
	q := url.Values{"isHidden": {"false"}}
	var t []Task
	if err := c.do(ctx, http.MethodGet, "/ScheduledTasks", q, nil, &t); err != nil {
		return nil, err
	}
	sort.SliceStable(t, func(i, j int) bool {
		if t[i].Category != t[j].Category {
			return t[i].Category < t[j].Category
		}
		return t[i].Name < t[j].Name
	})
	return t, nil
}

// StartTask runs a scheduled task now.
func (c *Client) StartTask(ctx context.Context, taskID string) error {
	return c.do(ctx, http.MethodPost, "/ScheduledTasks/Running/"+url.PathEscape(taskID), nil, nil, nil)
}

// StopTask cancels a running scheduled task.
func (c *Client) StopTask(ctx context.Context, taskID string) error {
	return c.do(ctx, http.MethodDelete, "/ScheduledTasks/Running/"+url.PathEscape(taskID), nil, nil, nil)
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
