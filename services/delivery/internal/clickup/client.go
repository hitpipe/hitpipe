// Package clickup maps a Delivery worker's protected requests to the ClickUp API.
package clickup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
)

var (
	ErrInvalidTarget   = errors.New("invalid ClickUp target")
	ErrCreateUnknown   = errors.New("ClickUp create outcome is unknown; do not retry")
	ErrReadUnavailable = errors.New("ClickUp task read unavailable")
	ErrMismatch        = errors.New("ClickUp task differs from the accepted target")

	numericID = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
	taskID    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
)

const apiOrigin = "https://api.clickup.com"
const maxResponseBytes = 256 * 1024

// Target is the exact accepted ClickUp assignment and rendered content.
// Resolve it from the fenced intent and check the current binding before use.
type Target struct {
	WorkspaceID   string
	ListID        string
	AssigneeID    string
	Title         string
	RenderedBlock string
}

// Start must be a protected, bounded network start for the current workspace,
// grant and epoch. It must use a transport without redirects or automatic retries.
type Start func(*http.Request) (*http.Response, error)

type Client struct {
	start Start
}

func New(start Start) (*Client, error) {
	if start == nil {
		return nil, errors.New("ClickUp protected start is required")
	}
	return &Client{start: start}, nil
}

func (t Target) assignee() (int64, error) {
	if !numericID.MatchString(t.WorkspaceID) || !numericID.MatchString(t.ListID) ||
		!numericID.MatchString(t.AssigneeID) || t.Title == "" || t.RenderedBlock == "" {
		return 0, ErrInvalidTarget
	}
	id, err := strconv.ParseInt(t.AssigneeID, 10, 64)
	if err != nil {
		return 0, ErrInvalidTarget
	}
	return id, nil
}

// Create calls the provider once. The worker must commit its write fence before
// calling it. The returned ID is evidence of identity, not of a verified result.
func (c *Client) Create(ctx context.Context, authorization string, target Target) (string, error) {
	assignee, err := target.assignee()
	if err != nil || authorization == "" {
		return "", ErrInvalidTarget
	}
	payload, err := json.Marshal(createRequest{target.Title, target.RenderedBlock, []int64{assignee}})
	if err != nil {
		return "", ErrInvalidTarget
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiOrigin+"/api/v2/list/"+target.ListID+"/task", bytes.NewReader(payload))
	if err != nil {
		return "", ErrInvalidTarget
	}
	req.GetBody = nil
	req.Close = true
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", "application/json")

	var observed struct {
		ID string `json:"id"`
	}
	if err := c.readJSON(req, &observed); err != nil || !taskID.MatchString(observed.ID) {
		return "", ErrCreateUnknown
	}
	return observed.ID, nil
}

type createRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Assignees   []int64 `json:"assignees"`
}

// Verify checks only the known native ID. It never searches for a replacement
// task or attempts a write. Missing or malformed attributes are not confirmed.
func (c *Client) Verify(ctx context.Context, authorization, knownID string, target Target) error {
	expectedAssignee, err := target.assignee()
	if err != nil || !taskID.MatchString(knownID) || authorization == "" {
		return ErrInvalidTarget
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiOrigin+"/api/v2/task/"+knownID, nil)
	if err != nil {
		return ErrInvalidTarget
	}
	req.Close = true
	req.Header.Set("Authorization", authorization)
	var observed readback
	if err := c.readJSON(req, &observed); err != nil {
		return ErrReadUnavailable
	}
	if observed.ID != knownID || string(observed.Workspace) != target.WorkspaceID ||
		string(observed.List.ID) != target.ListID || len(observed.Assignees) != 1 ||
		observed.Assignees[0].ID != expectedAssignee ||
		observed.Name != target.Title || observed.Description != target.RenderedBlock {
		return ErrMismatch
	}
	return nil
}

type readback struct {
	ID        string      `json:"id"`
	Workspace json.Number `json:"team_id"`
	List      struct {
		ID json.Number `json:"id"`
	} `json:"list"`
	Assignees []struct {
		ID int64 `json:"id"`
	} `json:"assignees"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (c *Client) readJSON(req *http.Request, dst any) error {
	resp, err := c.start(req)
	if err != nil {
		return err
	}
	if resp == nil || resp.Body == nil {
		return ErrReadUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrReadUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(raw) > maxResponseBytes {
		return ErrReadUnavailable
	}
	return json.Unmarshal(raw, dst)
}
