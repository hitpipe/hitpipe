// Package signingsecrets provides a file-backed diagnostic secret adapter.
// Production secure-store integration is a separate future adapter.
package signingsecrets

import (
	"context"
	"encoding/json"
	"errors"
	"os"
)

// FileResolver reads an installation-to-secret map from a local synthetic file.
type FileResolver struct {
	path string
}

// NewFileResolver creates a resolver with a non-empty path.
func NewFileResolver(path string) (FileResolver, error) {
	if path == "" {
		return FileResolver{}, errors.New("secret fixture path is required")
	}
	return FileResolver{path: path}, nil
}

// Resolve returns only the requested installation secret and hides file errors
// from the application layer. The caller must keep the file synthetic in this lab.
func (r FileResolver) Resolve(ctx context.Context, installationID string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if installationID == "" {
		return nil, errors.New("installation is required")
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		return nil, errors.New("secret unavailable")
	}
	var secrets map[string]string
	if json.Unmarshal(data, &secrets) != nil {
		return nil, errors.New("secret unavailable")
	}
	secret, ok := secrets[installationID]
	if !ok || secret == "" {
		return nil, errors.New("secret unavailable")
	}
	return []byte(secret), nil
}
