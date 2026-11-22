package workspaces

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/workspaces"
)

const maxWorkspaceItems = 50

type workspaceList struct {
	Items []app.Workspace `json:"items"`
}

type createInput struct {
	Name    string `json:"name"`
	Command struct {
		RequestID        string         `json:"request_id"`
		ExpectedRevision jsontext.Value `json:"expected_revision"`
	} `json:"command"`
}

func parseCommand(body []byte) (app.Command, bool) {
	var input createInput
	// The strict decoder rejects duplicate names, invalid UTF-8 and lone UTF-16
	// surrogates, including within escaped JSON strings. Field names are exact.
	if jsonv2.Unmarshal(body, &input, jsonv2.RejectUnknownMembers(true)) != nil ||
		!validWorkspaceName(input.Name) || !runtimeaccess.ValidUUID(input.Command.RequestID) {
		return app.Command{}, false
	}
	command := app.Command{RequestID: input.Command.RequestID, Name: input.Name}
	if len(input.Command.ExpectedRevision) > 0 {
		var revision *int64
		if jsonv2.Unmarshal(input.Command.ExpectedRevision, &revision) != nil || revision == nil || !validRevision(*revision) {
			return app.Command{}, false
		}
		command.ExpectedRevision = revision
	}
	return command, true
}

func validWorkspaceName(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) >= 1 && utf8.RuneCountInString(value) <= 120
}

func validRevision(value int64) bool {
	return value >= 1 && value <= authorizeaction.MaxWireInteger
}

func validWorkspace(value app.Workspace) bool {
	if !runtimeaccess.ValidUUID(value.WorkspaceID) || !validWorkspaceName(value.Name) ||
		!validRevision(value.Revision) || !validRevision(value.SecurityEpoch) {
		return false
	}
	switch value.Role {
	case "OWNER", "OPERATOR", "READER":
	default:
		return false
	}
	return app.IsPublicLifecycle(value.Lifecycle)
}

func correlationID(request *http.Request) (string, bool) {
	values := request.Header.Values("X-Request-ID")
	if len(values) == 0 && request.Method != http.MethodPost {
		return newRequestID(), true
	}
	if len(values) != 1 || !runtimeaccess.ValidUUID(values[0]) {
		return newRequestID(), false
	}
	return values[0], true
}

func secretHeader(request *http.Request, name string, minimum int) (string, bool) {
	values := request.Header.Values(name)
	if len(values) != 1 || len(values[0]) < minimum || len(values[0]) > 128 {
		return "", false
	}
	for _, char := range values[0] {
		if char <= ' ' || char > '~' || char == ',' {
			return "", false
		}
	}
	return values[0], true
}

func carriesAuthorityHeaders(request *http.Request) bool {
	for name := range request.Header {
		name = strings.ToLower(name)
		if strings.HasPrefix(name, "x-hitpipe-") && name != "x-hitpipe-session" {
			return true
		}
		switch name {
		case "authorization", "x-user-id", "x-user-role", "x-principal-id", "x-workspace-id", "x-forwarded-client-cert":
			return true
		}
	}
	return false
}
