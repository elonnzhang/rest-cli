package client

import (
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
