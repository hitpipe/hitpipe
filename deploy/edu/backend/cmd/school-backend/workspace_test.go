package main

import (
	"os"
	"path/filepath"
	"testing"
)

const personalWorkspace = "403163b4-24d3-48c3-a887-e7878be3b9a4"
const otherWorkspace = "e05d0bc4-2bf2-4fe2-84ee-08fa6f1145ee"

func putWorkspaceFile(t *testing.T, state, id string) string {
	t.Helper()
	path := filepath.Join(state, schoolWorkspaceFile)
	if err := os.WriteFile(path, []byte(id+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSchoolWorkspaceLegacy(t *testing.T) {
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(state, "identity.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	config := putWorkspaceFile(t, state, personalWorkspace)
	if _, err := bindWorkspace(state, config, personalWorkspace); err == nil {
		t.Fatal("existing identity changed workspace")
	}
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	if got, err := bindWorkspace(state, "", ""); got != workspaceID || err != nil {
		t.Fatalf("legacy workspace = %q, %v", got, err)
	}
}

func TestSchoolWorkspaceOwnershipAndRestart(t *testing.T) {
	state := t.TempDir()
	config := putWorkspaceFile(t, state, personalWorkspace)
	if _, err := bindWorkspace(state, config, otherWorkspace); err == nil {
		t.Fatal("launcher mismatch")
	}
	for range 2 {
		if got, err := bindWorkspace(state, config, personalWorkspace); got != personalWorkspace || err != nil {
			t.Fatalf("workspace = %q, %v", got, err)
		}
	}
	if _, err := bindWorkspace(state, "", ""); err == nil {
		t.Fatal("missing private scope fell back to the historical workspace")
	}
	putWorkspaceFile(t, state, otherWorkspace)
	if _, err := bindWorkspace(state, config, otherWorkspace); err == nil {
		t.Fatal("existing space was reassigned")
	}
	if got, err := readWorkspaceFile(filepath.Join(state, boundWorkspaceFile)); got != personalWorkspace || err != nil {
		t.Fatalf("workspace binding = %q, %v", got, err)
	}
}

func TestSchoolWorkspaceRejectsMalformedOrSymlinkedInput(t *testing.T) {
	for _, id := range []string{workspaceID, nilWorkspaceID, " " + personalWorkspace, personalWorkspace + "\n"} {
		state := t.TempDir()
		if _, err := resolveWorkspace(state, putWorkspaceFile(t, state, id)); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
	state := t.TempDir()
	path := filepath.Join(state, schoolWorkspaceFile)
	if err := os.Symlink(putWorkspaceFile(t, t.TempDir(), personalWorkspace), path); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveWorkspace(state, path); err == nil {
		t.Fatal("accepted symbolic link as workspace provision")
	}
}
