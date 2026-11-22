package profileupdate

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/profileupdate"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

var errTooLarge = errors.New("profile update HTTP: body exceeds size limit")

type updateInput struct {
	Command struct {
		RequestID        string `json:"request_id"`
		ExpectedRevision int64  `json:"expected_revision"`
	} `json:"command"`
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
}

func readCommand(w http.ResponseWriter, r *http.Request, control *http.ResponseController, requestID string) (app.Command, error) {
	contentType, present := singleHeader(r, "Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if !present || err != nil || !strings.EqualFold(mediaType, "application/json") ||
		len(r.Header.Values("Content-Encoding")) != 0 || r.Body == nil {
		return app.Command{}, app.ErrInvalid
	}
	if r.ContentLength > privateiam.MaxRequestBytes {
		return app.Command{}, errTooLarge
	}
	if r.Context().Err() != nil || control.SetReadDeadline(deadline(r.Context())) != nil {
		return app.Command{}, app.ErrUnavailable
	}
	defer func() { _ = control.SetReadDeadline(time.Time{}) }()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, privateiam.MaxRequestBytes))
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		return app.Command{}, errTooLarge
	}
	if err != nil || r.Context().Err() != nil {
		return app.Command{}, app.ErrUnavailable
	}

	var input updateInput
	// The strict decoder rejects duplicate/unknown fields, invalid Unicode
	// and extra JSON values. Null and absent fields cannot pass these bounds.
	if jsonv2.Unmarshal(body, &input, jsonv2.RejectUnknownMembers(true)) != nil ||
		!validText(input.DisplayName, 120) || !validText(input.Timezone, 80) ||
		!validRevision(input.Command.ExpectedRevision) || !runtimeaccess.ValidUUID(input.Command.RequestID) ||
		!strings.EqualFold(requestID, input.Command.RequestID) {
		return app.Command{}, app.ErrInvalid
	}
	if values := r.Header.Values("If-Match"); len(values) > 0 {
		tag := strconv.Quote(strconv.FormatInt(input.Command.ExpectedRevision, 10))
		if len(values) != 1 || strings.TrimSpace(values[0]) != tag {
			return app.Command{}, app.ErrStale
		}
	}
	return app.Command{RequestID: input.Command.RequestID, ExpectedRevision: input.Command.ExpectedRevision,
		DisplayName: input.DisplayName, Timezone: input.Timezone}, nil
}

func validText(value string, max int) bool {
	return utf8.ValidString(value) && value != "" && utf8.RuneCountInString(value) <= max
}

func validRevision(value int64) bool {
	return value > 0 && value <= authorizeaction.MaxWireInteger
}

func validProfile(profile getprofile.Profile) bool {
	return runtimeaccess.ValidUUID(profile.UserID) && validRevision(profile.Revision) &&
		validText(profile.DisplayName, 120) && validText(profile.Timezone, 80)
}

func singleHeader(r *http.Request, name string) (string, bool) {
	values := r.Header.Values(name)
	if len(values) != 1 {
		return "", false
	}
	return values[0], true
}

func secretHeader(r *http.Request, name string, minimum int) (string, bool) {
	value, ok := singleHeader(r, name)
	if !ok || len(value) < minimum || len(value) > 128 {
		return "", false
	}
	for _, char := range value {
		if char <= ' ' || char > '~' || char == ',' {
			return "", false
		}
	}
	return value, true
}

func hasAuthorityHeaders(r *http.Request) bool {
	for name := range r.Header {
		name = strings.ToLower(name)
		if strings.HasPrefix(name, "x-hitpipe-") && name != "x-hitpipe-session" {
			return true
		}
		switch name {
		case "authorization", "proxy-authorization", "x-principal-id", "x-user-id", "x-user-role",
			"x-workspace-id", "x-forwarded-client-cert":
			return true
		}
	}
	return false
}
