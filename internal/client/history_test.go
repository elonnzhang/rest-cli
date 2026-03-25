package client_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elonnzhang/rest-cli/internal/client"
)

func TestHistorySaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")

	h := client.NewHistory(10)
	h.Add(client.HistoryEntry{Method: "GET", URL: "https://a.com", StatusCode: 200, Duration: time.Millisecond})
	h.Add(client.HistoryEntry{Method: "POST", URL: "https://b.com", StatusCode: 201, Duration: 2 * time.Millisecond})

	if err := h.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := client.LoadHistory(path, 10)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}

	entries := loaded.Entries()
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	// Most-recent first (POST was added last)
	if entries[0].URL != "https://b.com" || entries[0].Method != "POST" {
		t.Errorf("entries[0] = %+v, want POST https://b.com", entries[0])
	}
	if entries[1].URL != "https://a.com" || entries[1].Method != "GET" {
		t.Errorf("entries[1] = %+v, want GET https://a.com", entries[1])
	}
}

func TestHistoryLoadMissing(t *testing.T) {
	h, err := client.LoadHistory("/nonexistent/path/history.json", 10)
	if err != nil {
		t.Fatalf("LoadHistory missing: unexpected error: %v", err)
	}
	if len(h.Entries()) != 0 {
		t.Errorf("expected empty history, got %d entries", len(h.Entries()))
	}
}

func TestHistoryLoadCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	if err := os.WriteFile(path, []byte("not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	h, err := client.LoadHistory(path, 10)
	if err != nil {
		t.Fatalf("LoadHistory corrupt: unexpected error: %v", err)
	}
	if len(h.Entries()) != 0 {
		t.Errorf("expected empty history, got %d entries", len(h.Entries()))
	}
}

func TestHistory(t *testing.T) {
	t.Run("records entries and retrieves them in reverse order", func(t *testing.T) {
		h := client.NewHistory(10)
		h.Add(client.HistoryEntry{Method: "GET", URL: "https://a.com", StatusCode: 200, Duration: time.Millisecond})
		h.Add(client.HistoryEntry{Method: "POST", URL: "https://b.com", StatusCode: 201, Duration: 2 * time.Millisecond})

		entries := h.Entries()
		if len(entries) != 2 {
			t.Fatalf("got %d entries, want 2", len(entries))
		}
		// Most recent first
		if entries[0].URL != "https://b.com" {
			t.Errorf("entries[0].URL = %q, want https://b.com", entries[0].URL)
		}
		if entries[1].URL != "https://a.com" {
			t.Errorf("entries[1].URL = %q, want https://a.com", entries[1].URL)
		}
	})

	t.Run("evicts oldest entry when capacity exceeded", func(t *testing.T) {
		h := client.NewHistory(3)
		for i := 0; i < 4; i++ {
			h.Add(client.HistoryEntry{URL: string(rune('a' + i))})
		}
		entries := h.Entries()
		if len(entries) != 3 {
			t.Fatalf("got %d entries, want 3", len(entries))
		}
		// Oldest ("a") should be evicted; most recent ("d") first
		if entries[0].URL != "d" {
			t.Errorf("entries[0].URL = %q, want d", entries[0].URL)
		}
		if entries[2].URL != "b" {
			t.Errorf("entries[2].URL = %q, want b", entries[2].URL)
		}
	})

	t.Run("101 entries evicts oldest (ring buffer)", func(t *testing.T) {
		h := client.NewHistory(100)
		for i := 0; i < 101; i++ {
			h.Add(client.HistoryEntry{StatusCode: i})
		}
		entries := h.Entries()
		if len(entries) != 100 {
			t.Fatalf("got %d entries, want 100", len(entries))
		}
		// Most recent (100) should be first
		if entries[0].StatusCode != 100 {
			t.Errorf("entries[0].StatusCode = %d, want 100", entries[0].StatusCode)
		}
		// Oldest (1) should be last (0 was evicted)
		if entries[99].StatusCode != 1 {
			t.Errorf("entries[99].StatusCode = %d, want 1", entries[99].StatusCode)
		}
	})
}
