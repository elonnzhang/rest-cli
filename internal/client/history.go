package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// HistoryEntry records a single executed request and its outcome.
type HistoryEntry struct {
	Timestamp  time.Time
	Method     string
	URL        string
	StatusCode int
	Duration   time.Duration
}

// History is a fixed-capacity ring buffer of HistoryEntry values.
type History struct {
	entries  []HistoryEntry
	capacity int
	head     int // index of the next write slot
	count    int
}

// NewHistory creates a History that holds at most cap entries.
func NewHistory(cap int) *History {
	return &History{
		entries:  make([]HistoryEntry, cap),
		capacity: cap,
	}
}

// Add appends an entry. If the buffer is full, the oldest entry is overwritten.
func (h *History) Add(e HistoryEntry) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	h.entries[h.head] = e
	h.head = (h.head + 1) % h.capacity
	if h.count < h.capacity {
		h.count++
	}
}

// Save writes all history entries to path as a JSON array (oldest first).
// Creates the parent directory if it does not exist.
// Uses an atomic write (temp file + rename) to avoid partial writes.
func (h *History) Save(path string) error {
	entries := h.Entries()
	// Entries() returns most-recent first. Reverse to oldest-first for stable serialization.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // clean up orphaned temp file on rename failure
		return err
	}
	return nil
}

// LoadHistory reads a history file from path and returns a History with the given capacity.
// If the file is missing or contains invalid JSON, returns a fresh empty History with nil error.
func LoadHistory(path string, cap int) (*History, error) {
	if cap <= 0 {
		cap = 1
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return NewHistory(cap), nil
	}
	var entries []HistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return NewHistory(cap), nil
	}
	h := NewHistory(cap)
	// entries is oldest-first; Add() each in order to reconstruct the ring buffer correctly.
	for _, e := range entries {
		h.Add(e)
	}
	return h, nil
}

// Entries returns all entries in reverse chronological order (most recent first).
func (h *History) Entries() []HistoryEntry {
	result := make([]HistoryEntry, h.count)
	for i := 0; i < h.count; i++ {
		// Walk backwards from (head-1)
		idx := (h.head - 1 - i + h.capacity) % h.capacity
		result[i] = h.entries[idx]
	}
	return result
}
