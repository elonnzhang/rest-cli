package tui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- buildEditorArgs tests ---

func TestBuildEditorArgs_DefaultEditor(t *testing.T) {
	binary, args := buildEditorArgs("", "/some/file.http", 1)
	if binary != "vi" {
		t.Errorf("binary = %q, want vi", binary)
	}
	if len(args) != 1 || args[0] != "/some/file.http" {
		t.Errorf("args = %v, want [/some/file.http]", args)
	}
}

func TestBuildEditorArgs_MultiWordEditor(t *testing.T) {
	binary, args := buildEditorArgs("code --wait", "/some/file.http", 1)
	if binary != "code" {
		t.Errorf("binary = %q, want code", binary)
	}
	// args: [--wait, /some/file.http] (no +1 because line == 1)
	if len(args) != 2 || args[0] != "--wait" || args[1] != "/some/file.http" {
		t.Errorf("args = %v, want [--wait /some/file.http]", args)
	}
}

func TestBuildEditorArgs_LineJump(t *testing.T) {
	binary, args := buildEditorArgs("nvim", "/some/file.http", 42)
	if binary != "nvim" {
		t.Errorf("binary = %q, want nvim", binary)
	}
	// args: [+42, /some/file.http]
	if len(args) != 2 || args[0] != "+42" || args[1] != "/some/file.http" {
		t.Errorf("args = %v, want [+42 /some/file.http]", args)
	}
}

func TestBuildEditorArgs_LineOne(t *testing.T) {
	// line == 1: skip +1 (most editors default to line 1)
	_, args := buildEditorArgs("vim", "/some/file.http", 1)
	for _, a := range args {
		if a == "+1" {
			t.Errorf("unexpected +1 argument in args %v", args)
		}
	}
	if args[len(args)-1] != "/some/file.http" {
		t.Errorf("last arg = %q, want /some/file.http", args[len(args)-1])
	}
}

// --- editorClosedMsg handler tests ---

// newTestModel builds a minimal Model with a single .http file loaded.
func newTestModel(t *testing.T, httpContent string) (Model, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.http")
	if err := os.WriteFile(path, []byte(httpContent), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{
		Files: []string{path},
		Env:   map[string]string{},
	})
	m.width = 80
	m.height = 24
	// Expand the file so requests are visible in the flat tree.
	m.toggleExpand(path)
	// Move cursor to the first request row (index 1).
	if len(m.flatTree()) > 1 {
		m.cursor = 1
	}
	m.loadSelectedRequest()
	return m, path
}

func TestEditorClosedMsg_Error(t *testing.T) {
	m, path := newTestModel(t, "### GET example\nGET https://example.com HTTP/1.1\n")
	boom := errors.New("editor not found")

	result, cmd := m.Update(editorClosedMsg{path: path, err: boom, openedAt: time.Now()})
	if cmd != nil {
		t.Errorf("expected nil cmd on error, got non-nil")
	}
	rm := result.(Model)
	if rm.selectedErr == nil || rm.selectedErr.Error() != boom.Error() {
		t.Errorf("selectedErr = %v, want %v", rm.selectedErr, boom)
	}
}

func TestEditorClosedMsg_NoChange(t *testing.T) {
	m, path := newTestModel(t, "### GET example\nGET https://example.com HTTP/1.1\n")
	// openedAt is in the future relative to the file's actual mtime → no rerun
	openedAt := time.Now().Add(10 * time.Second)

	result, cmd := m.Update(editorClosedMsg{path: path, err: nil, openedAt: openedAt})
	if cmd != nil {
		t.Errorf("expected nil cmd (no change), got non-nil")
	}
	rm := result.(Model)
	if rm.selectedErr != nil {
		t.Errorf("unexpected selectedErr: %v", rm.selectedErr)
	}
}

func TestEditorClosedMsg_Changed(t *testing.T) {
	m, path := newTestModel(t, "### GET example\nGET https://example.com HTTP/1.1\n")
	// openedAt is before the file's mtime → should trigger rerun
	openedAt := time.Time{} // zero time is always before any real mtime

	result, cmd := m.Update(editorClosedMsg{path: path, err: nil, openedAt: openedAt})
	_ = result
	// executeSelected returns a Cmd (the HTTP request); just verify it's non-nil.
	if cmd == nil {
		t.Errorf("expected a Cmd (auto-rerun) when file changed, got nil")
	}
}

func TestEditorClosedMsg_ParseError(t *testing.T) {
	m, _ := newTestModel(t, "### GET example\nGET https://example.com HTTP/1.1\n")
	// Use a file that no longer exists to force a parse (open) error.
	missingPath := filepath.Join(t.TempDir(), "missing.http")

	result, cmd := m.Update(editorClosedMsg{path: missingPath, err: nil, openedAt: time.Time{}})
	if cmd != nil {
		t.Errorf("expected nil cmd on parse error, got non-nil")
	}
	rm := result.(Model)
	if rm.selectedErr == nil {
		t.Errorf("expected selectedErr after parse failure, got nil")
	}
}

func TestEditorClosedMsg_CursorClamp(t *testing.T) {
	// Start with 2 requests, then simulate editing the file down to 1 request.
	dir := t.TempDir()
	path := filepath.Join(dir, "test.http")
	twoReqs := "### First\nGET https://a.com HTTP/1.1\n\n###\nGET https://b.com HTTP/1.1\n"
	if err := os.WriteFile(path, []byte(twoReqs), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{Files: []string{path}, Env: map[string]string{}})
	m.width = 80
	m.height = 24
	m.toggleExpand(path)
	// Position cursor at last request row
	items := m.flatTree()
	m.cursor = len(items) - 1
	m.loadSelectedRequest()

	// Now overwrite the file with only one request (simulate user deleting the second).
	oneReq := "### First\nGET https://a.com HTTP/1.1\n"
	if err := os.WriteFile(path, []byte(oneReq), 0o644); err != nil {
		t.Fatal(err)
	}

	result, _ := m.Update(editorClosedMsg{path: path, err: nil, openedAt: time.Time{}})
	rm := result.(Model)
	if rm.cursor >= len(rm.flatTree()) {
		t.Errorf("cursor %d is out of bounds (tree len %d)", rm.cursor, len(rm.flatTree()))
	}
}

func TestEditorClosedMsg_FileCollapsed(t *testing.T) {
	// File is NOT expanded — handler must still re-parse (not gate on expansion).
	dir := t.TempDir()
	path := filepath.Join(dir, "test.http")
	if err := os.WriteFile(path, []byte("### GET\nGET https://example.com HTTP/1.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(Config{Files: []string{path}, Env: map[string]string{}})
	m.width = 80
	m.height = 24
	// Do NOT expand — file stays collapsed.

	result, _ := m.Update(editorClosedMsg{path: path, err: nil, openedAt: time.Now().Add(10 * time.Second)})
	rm := result.(Model)
	// After the handler, the file's requests should be cached (re-parsed).
	if _, ok := rm.fileReqs[path]; !ok {
		t.Errorf("expected fileReqs[path] populated after editorClosedMsg, but missing")
	}
	if rm.selectedErr != nil {
		t.Errorf("unexpected selectedErr: %v", rm.selectedErr)
	}
}
