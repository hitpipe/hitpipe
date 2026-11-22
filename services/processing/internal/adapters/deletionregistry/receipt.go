package deletionregistry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

func (c *Client) Submit(ctx context.Context, r Receipt) error {
	if c == nil || ctx == nil || ctx.Err() != nil || r.Environment != "test" ||
		r.WorkspaceID != c.provision.WorkspaceID || r.Generation != c.provision.Generation ||
		!id(r.DeletionID) || !version(r.Revision) || !digestPattern.MatchString(r.ScopeDigest) ||
		r.AppliedAt.IsZero() || !digestPattern.MatchString(r.ApplicationRef) {
		return ErrUnavailable
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return ErrUnavailable
	}
	u := c.endpoint
	u.Path, u.RawQuery = "/internal/maintenance/v1/case-deletion/receipt", ""
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), io.NopCloser(bytes.NewReader(raw)))
	if err != nil {
		return ErrUnavailable
	}
	req.ContentLength = int64(len(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || ctx.Err() != nil {
		return ErrUnavailable
	}
	return nil
}
