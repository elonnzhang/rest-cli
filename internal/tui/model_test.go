package tui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elonnzhang/rest-cli/internal/client"
	"github.com/elonnzhang/rest-cli/internal/tui"
)

// newTestModel creates a model with a set of .http files for testing.
func newTestModel(files []string) tui.Model {
	return tui.New(tui.Config{
		Files:   files,
		History: client.NewHistory(100),
	})
}

func TestModel_Update(t *testing.T) {
	t.Run("navigate down moves selection", func(t *testing.T) {
		files := []string{"auth.http", "users.http", "products.http"}
		m := newTestModel(files)

		m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		next := m2.(tui.Model)
		if next.SelectedFileIndex() == m.SelectedFileIndex() {
			t.Error("expected selection to move down")
		}
	})

	t.Run("navigate up at top does not go negative", func(t *testing.T) {
		files := []string{"auth.http", "users.http"}
		m := newTestModel(files)
		// Already at 0, pressing up should stay at 0
		m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		next := m2.(tui.Model)
		if next.SelectedFileIndex() < 0 {
			t.Error("index should not go below 0")
		}
	})

	t.Run("slash activates search mode", func(t *testing.T) {
		m := newTestModel([]string{"auth.http"})
		m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		next := m2.(tui.Model)
		if !next.Searching() {
			t.Error("expected search mode to activate on '/'")
		}
	})

	t.Run("esc clears search mode", func(t *testing.T) {
		m := newTestModel([]string{"auth.http"})
		m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m3, _ := m2.(tui.Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
		if m3.(tui.Model).Searching() {
			t.Error("expected search mode to clear on Esc")
		}
	})

	t.Run("enter while in-flight is a no-op", func(t *testing.T) {
		m := newTestModel([]string{"auth.http"})
		m = m.WithInFlight(true)

		m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		next := m2.(tui.Model)
		// inFlight stays true and no new Cmd is dispatched (cmd is nil)
		if !next.InFlight() {
			t.Error("expected inFlight to remain true")
		}
		if cmd != nil {
			t.Error("expected no command dispatched while in-flight")
		}
	})

	t.Run("ctrl+c while idle returns tea.Quit", func(t *testing.T) {
		m := newTestModel([]string{"auth.http"})
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if cmd == nil {
			t.Fatal("expected a command from Ctrl+C when idle")
		}
		// Execute the command and check it produces tea.QuitMsg
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", msg)
		}
	})

	t.Run("ctrl+c while in-flight cancels request, does not quit", func(t *testing.T) {
		m := newTestModel([]string{"auth.http"})
		cancelled := false
		m = m.WithCancelFunc(func() { cancelled = true })
		m = m.WithInFlight(true)

		m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		next := m2.(tui.Model)

		if !cancelled {
			t.Error("expected cancel function to be called")
		}
		if next.InFlight() {
			t.Error("expected inFlight to be cleared after cancel")
		}
		if cmd != nil {
			t.Error("expected no quit command when cancelling in-flight request")
		}
	})

	t.Run("ResponseMsg updates response pane", func(t *testing.T) {
		m := newTestModel([]string{"auth.http"})
		m = m.WithInFlight(true)

		resp := &client.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Body:       `{"ok":true}`,
		}
		m2, _ := m.Update(tui.ResponseMsg{Response: resp})
		next := m2.(tui.Model)

		if next.InFlight() {
			t.Error("expected inFlight to be cleared after response")
		}
		if next.LastResponse() == nil {
			t.Error("expected LastResponse to be set")
		}
		if next.LastResponse().StatusCode != 200 {
			t.Errorf("expected status 200, got %d", next.LastResponse().StatusCode)
		}
	})

	t.Run("ResponseMsg with error stores error", func(t *testing.T) {
		m := newTestModel([]string{"auth.http"})
		m = m.WithInFlight(true)

		m2, _ := m.Update(tui.ResponseMsg{Err: errTest})
		next := m2.(tui.Model)

		if next.InFlight() {
			t.Error("expected inFlight to be cleared")
		}
		if next.LastErr() == nil {
			t.Error("expected error to be stored")
		}
	})

	t.Run("window resize stores dimensions", func(t *testing.T) {
		m := newTestModel([]string{"auth.http"})
		m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		next := m2.(tui.Model)
		w, h := next.Size()
		if w != 120 || h != 40 {
			t.Errorf("expected 120x40, got %dx%d", w, h)
		}
	})

	t.Run("file discovery msg populates file list", func(t *testing.T) {
		m := newTestModel(nil)
		found := []string{"a.http", "b.http", "c.http"}
		m2, _ := m.Update(tui.FilesDiscoveredMsg{Files: found})
		next := m2.(tui.Model)
		if len(next.Files()) != 3 {
			t.Errorf("expected 3 files, got %d", len(next.Files()))
		}
	})
}

// sentinel error for tests
var errTest = &testError{"test error"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
