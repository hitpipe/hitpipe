package clickup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

var demoTarget = Target{"101", "202", "303", "Testing", "[HITPIPE-HANDOFF:demo:v1]\nSynthetic"}

func TestWriteThenVerifyExactKnownTask(t *testing.T) {
	calls := 0
	client, err := New(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "api.clickup.com" || req.Header.Get("Authorization") != "synthetic-token" {
			t.Fatal("unexpected destination or authorization")
		}
		switch calls {
		case 1:
			if req.Method != http.MethodPost || req.URL.Path != "/api/v2/list/202/task" || req.GetBody != nil {
				t.Fatal("create must use the one permitted list")
			}
			body, _ := io.ReadAll(req.Body)
			if string(body) != `{"name":"Testing","description":"[HITPIPE-HANDOFF:demo:v1]\nSynthetic","assignees":[303]}` {
				t.Fatalf("unexpected create payload: %s", body)
			}
			return response(http.StatusOK, `{"id":"v1_Task"}`), nil
		case 2:
			if req.Method != http.MethodGet || req.URL.Path != "/api/v2/task/v1_Task" {
				t.Fatal("read must use the exact ID from the write")
			}
			return response(http.StatusOK, task("101", "202", 303, "Synthetic")), nil
		default:
			t.Fatal("unexpected provider request")
			return nil, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := client.Create(context.Background(), "synthetic-token", demoTarget)
	if err != nil || id != "v1_Task" {
		t.Fatalf("native ID not established: %q %v", id, err)
	}
	if err := client.Verify(context.Background(), "synthetic-token", id, demoTarget); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("unexpected request count %d", calls)
	}
}

func TestCreateUnknownDoesNotRetry(t *testing.T) {
	for _, example := range []struct {
		status int
		body   string
		err    error
	}{
		{http.StatusTemporaryRedirect, "", nil},
		{http.StatusServiceUnavailable, "", nil},
		{http.StatusOK, `{"id":"../unsafe"}`, nil},
		{0, "", io.ErrUnexpectedEOF},
	} {
		calls := 0
		client, _ := New(func(*http.Request) (*http.Response, error) {
			calls++
			if example.err != nil {
				return nil, example.err
			}
			return response(example.status, example.body), nil
		})
		if id, err := client.Create(context.Background(), "synthetic-token", demoTarget); id != "" ||
			!errors.Is(err, ErrCreateUnknown) || calls != 1 {
			t.Fatalf("create was confirmed or repeated: id=%q err=%v calls=%d", id, err, calls)
		}
	}
}

func TestVerifyRejectsMismatch(t *testing.T) {
	for _, body := range []string{
		task("104", "202", 303, "Synthetic"),
		task("101", "204", 303, "Synthetic"),
		task("101", "202", 304, "Synthetic"),
		task("101", "202", 303, "Edited"),
		`{"id":"v1_Task"}`,
	} {
		client, _ := New(func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, body), nil
		})
		if err := client.Verify(context.Background(), "synthetic-token", "v1_Task", demoTarget); !errors.Is(err, ErrMismatch) {
			t.Fatalf("unverified target passed: %v", err)
		}
	}
}

func response(status int, raw string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(raw))}
}

func task(workspace, list string, assignee int, content string) string {
	return fmt.Sprintf(`{"id":"v1_Task","team_id":%q,"list":{"id":%q},"assignees":[{"id":%d}],"name":"Testing","description":%q}`, workspace, list, assignee, "[HITPIPE-HANDOFF:demo:v1]\n"+content)
}
