package dispatch

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/application/verifytarget"
	"github.com/dkar-dev/hitpipe/services/delivery/internal/domain/readback"
	"github.com/gowebpki/jcs"
)

const observationSource = "CLICKUP_GET_TASK_V2"

func (c *ClickUp) readRequest(id, token string) (*http.Request, error) {
	if !validToken(token) || !validTaskID(id) || strings.ContainsAny(id, "/\\%?#") || id == "." || id == ".." {
		return nil, ErrHeld
	}
	dst := c.origin
	dst.Path = "/api/v2/task/" + id
	req, err := http.NewRequest(http.MethodGet, dst.String(), nil)
	if err != nil {
		return nil, ErrHeld
	}
	req.Header.Set("Authorization", token)
	return req, nil
}

type targetReader struct {
	ex   *readExchange
	id   string
	code string
}

func (r *targetReader) Read(ctx context.Context, id string) (readback.Observation, verifytarget.Provenance, error) {
	if id != r.id {
		return readback.Observation{}, verifytarget.Provenance{}, ErrHeld
	}
	response, reusable, err := r.ex.receive(ctx)
	keep := false
	defer func() { r.ex.finish(keep && reusable && ctx.Err() == nil) }()
	proof := verifytarget.Provenance{Source: observationSource, ObservedAt: time.Now().UTC().Truncate(time.Microsecond)}
	r.code = "READ_FAILED"
	if err != nil {
		return readback.Observation{}, proof, err
	}
	switch response.status {
	case 403, 404:
		r.code = "READ_NOT_ACCESSIBLE"
	case 429:
		r.code = "READ_RATE_LIMITED"
	case 200:
		o, err := observedTask(response.body)
		if err != nil {
			r.code = "READ_INVALID_RESPONSE"
			return o, proof, err
		}
		if o.TargetID != id {
			r.code = "READ_ID_MISMATCH"
			return readback.Observation{}, proof, ErrHeld
		}
		r.code = "READ_OBSERVED"
		keep = true
		return o, proof, nil
	}
	return readback.Observation{}, proof, ErrHeld
}

type boundedResponse struct {
	status int
	header http.Header
	body   []byte
}

func (e *exchange) readBounded(ctx context.Context) (boundedResponse, error) {
	deadline, ok := ctx.Deadline()
	if !ok || ctx.Err() != nil || e.conn.SetDeadline(deadline) != nil {
		return boundedResponse{}, ErrHeld
	}
	stop := context.AfterFunc(ctx, func() { _ = e.conn.Close() })
	defer stop()
	res, err := http.ReadResponse(bufio.NewReader(&io.LimitedReader{R: e.conn, N: 2 * maxResponse}), e.req)
	if err != nil {
		return boundedResponse{}, ErrHeld
	}
	response := boundedResponse{status: res.StatusCode, header: res.Header}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return response, ErrHeld
	}
	response.body = data
	return response, nil
}

func observedTask(raw []byte) (readback.Observation, error) {
	var o readback.Observation
	if !utf8.Valid(raw) {
		return o, ErrHeld
	}
	if _, err := jcs.Transform(raw); err != nil {
		return o, ErrHeld
	}
	var f struct {
		ID   string          `json:"id"`
		Team json.RawMessage `json:"team_id"`
		List struct {
			ID json.RawMessage `json:"id"`
		} `json:"list"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Assignees   []struct {
			ID json.RawMessage `json:"id"`
		} `json:"assignees"`
	}
	// Read the original numeric lexemes. Re-encoding canonical JSON would
	// round provider-assigned integer IDs outside the JavaScript safe range.
	if json.Unmarshal(raw, &f) != nil || !validTaskID(f.ID) || len(f.Assignees) > 100 {
		return o, ErrHeld
	}
	o = readback.Observation{TargetID: f.ID, Retrieved: true, Workspace: providerID(f.Team), List: providerID(f.List.ID),
		Title: f.Name, HandoffBlock: f.Description}
	if len([]rune(o.Title)) > 255 {
		return readback.Observation{}, ErrHeld
	}
	if f.Assignees != nil {
		o.Assignees = make([]string, 0, len(f.Assignees))
		for _, a := range f.Assignees {
			id := providerID(a.ID)
			if id == "" {
				return readback.Observation{}, ErrHeld
			}
			o.Assignees = append(o.Assignees, id)
		}
	}
	return o, nil
}

func providerID(raw json.RawMessage) string {
	var id string
	if json.Unmarshal(raw, &id) == nil && validTaskID(id) {
		return id
	}
	if positiveDecimal(string(raw)) {
		return string(raw)
	}
	return ""
}

var errReadBudget = errors.New("read credential budget exhausted")
