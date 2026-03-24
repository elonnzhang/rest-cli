package tui_test

import (
	"testing"

	"github.com/elonnzhang/rest-cli/internal/tui"
)

func TestFuzzyFilter(t *testing.T) {
	files := []string{
		"auth/login.http",
		"auth/register.http",
		"users/list.http",
		"users/create.http",
		"products/search.http",
	}

	tests := []struct {
		name    string
		query   string
		files   []string
		wantMin int // minimum expected matches
		wantIn  []string
		wantOut []string
	}{
		{
			name:    "empty query returns all files",
			query:   "",
			files:   files,
			wantMin: 5,
			wantIn:  files,
		},
		{
			name:    "auth matches auth files",
			query:   "auth",
			files:   files,
			wantMin: 1,
			wantIn:  []string{"auth/login.http", "auth/register.http"},
			wantOut: []string{"products/search.http"},
		},
		{
			name:    "no matches returns empty slice",
			query:   "zzzznothing",
			files:   files,
			wantMin: 0,
			wantOut: files,
		},
		{
			name:    "partial fuzzy match",
			query:   "usrlist",
			files:   files,
			wantMin: 1,
			wantIn:  []string{"users/list.http"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tui.FuzzyFilter(tt.query, tt.files)

			if len(got) < tt.wantMin {
				t.Errorf("got %d results, want at least %d (query=%q)", len(got), tt.wantMin, tt.query)
			}

			// Check expected inclusions
			gotSet := make(map[string]bool, len(got))
			for _, g := range got {
				gotSet[g] = true
			}
			for _, want := range tt.wantIn {
				if !gotSet[want] {
					t.Errorf("expected %q in results for query=%q, got: %v", want, tt.query, got)
				}
			}
			// Check expected exclusions
			for _, notWant := range tt.wantOut {
				if gotSet[notWant] {
					t.Errorf("expected %q NOT in results for query=%q", notWant, tt.query)
				}
			}
		})
	}
}
