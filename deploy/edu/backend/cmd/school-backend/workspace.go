package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
)

const schoolWorkspaceFile = "workspace-id"
const boundWorkspaceFile = "workspace-bound-id"
const nilWorkspaceID = "00000000-0000-0000-0000-000000000000"

func readWorkspaceFile(path string) (string, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Getuid()) || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return "", errors.New("school workspace files must be private regular files owned by the supervisor")
	}
	content, err := io.ReadAll(io.LimitReader(file, 38))
	if err != nil {
		return "", err
	}
	id := strings.TrimSuffix(string(content), "\n")
	if !drafts.UUID(id) || id == nilWorkspaceID {
		return "", errors.New("school workspace file must contain one canonical workspace UUID")
	}
	return id, nil
}

func resolveWorkspace(state, config string) (string, error) {
	provisioned := filepath.Join(state, schoolWorkspaceFile)
	selected := workspaceID
	if config != "" {
		if config != provisioned {
			return "", errors.New("school workspace configuration must come from the private state directory")
		}
		value, err := readWorkspaceFile(provisioned)
		if err != nil {
			return "", err
		}
		if value == workspaceID {
			return "", errors.New("the historical fixture workspace UUID is reserved")
		}
		selected = value
	} else if _, err := os.Lstat(provisioned); !os.IsNotExist(err) {
		return "", errors.New("an explicitly provisioned school workspace requires HITPIPE_SCHOOL_WORKSPACE_FILE")
	}

	bound, err := readWorkspaceFile(filepath.Join(state, boundWorkspaceFile))
	if err == nil {
		if bound != selected {
			return "", errors.New("school state belongs to a different workspace")
		}
		return selected, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if _, err := os.Lstat(filepath.Join(state, "identity.json")); !os.IsNotExist(err) && selected != workspaceID {
		return "", errors.New("pre-existing school identity is bound to the historical fixture workspace")
	}
	return selected, nil
}

func bindWorkspace(state, config, expected string) (string, error) {
	selected, err := resolveWorkspace(state, config)
	if err != nil {
		return "", err
	}
	if (expected != "" || selected != workspaceID) && expected != selected {
		return "", errors.New("school workspace does not match the launcher provision")
	}
	path := filepath.Join(state, boundWorkspaceFile)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		if err := writePrivate(path, []byte(selected+"\n")); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return selected, nil
}
