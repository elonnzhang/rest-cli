package parser_test

import (
	"testing"

	"github.com/elonnzhang/rest-cli/internal/parser"
)

func TestLoadEnv(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]string
	}{
		{
			name:    "basic key=value",
			content: "FOO=bar\nBAZ=qux\n",
			want:    map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name:    "quoted values",
			content: `KEY="hello world"` + "\n",
			want:    map[string]string{"KEY": "hello world"},
		},
		{
			name:    "comment lines ignored",
			content: "# comment\nFOO=bar\n",
			want:    map[string]string{"FOO": "bar"},
		},
		{
			name:    "empty lines ignored",
			content: "\nFOO=bar\n\n",
			want:    map[string]string{"FOO": "bar"},
		},
		{
			name:    "values with equals sign",
			content: "TOKEN=abc=def==\n",
			want:    map[string]string{"TOKEN": "abc=def=="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.ParseEnv(tt.content)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, wv := range tt.want {
				gv, ok := got[k]
				if !ok {
					t.Errorf("missing key %q", k)
				} else if gv != wv {
					t.Errorf("key %q: got %q, want %q", k, gv, wv)
				}
			}
		})
	}
}

func TestSubstitute(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{
			name:  "simple substitution",
			input: "Bearer {{TOKEN}}",
			env:   map[string]string{"TOKEN": "abc123"},
			want:  "Bearer abc123",
		},
		{
			name:  "multiple substitutions",
			input: "{{HOST}}/{{PATH}}",
			env:   map[string]string{"HOST": "https://api.example.com", "PATH": "users"},
			want:  "https://api.example.com/users",
		},
		{
			name:  "unresolved variable left as-is",
			input: "Bearer {{MISSING}}",
			env:   map[string]string{},
			want:  "Bearer {{MISSING}}",
		},
		{
			name:    "circular reference returns error",
			input:   "{{FOO}}",
			env:     map[string]string{"FOO": "{{BAR}}", "BAR": "{{FOO}}"},
			wantErr: true,
		},
		{
			name: "precedence: later env overrides earlier",
			// Substitute honours whatever is in the env map;
			// precedence (--env > .env.local > .env) is enforced at load time by MergeEnv.
			input: "{{KEY}}",
			env:   map[string]string{"KEY": "override"},
			want:  "override",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.Substitute(tt.input, tt.env)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSubstituteJetBrains(t *testing.T) {
	tests := []struct {
		name  string
		input string
		env   map[string]string
		want  string
	}{
		{
			name:  "basic substitution",
			input: "Bearer $TOKEN",
			env:   map[string]string{"TOKEN": "abc123"},
			want:  "Bearer abc123",
		},
		{
			name:  "multiple vars",
			input: "$HOST/$PATH",
			env:   map[string]string{"HOST": "https://api.example.com", "PATH": "users"},
			want:  "https://api.example.com/users",
		},
		{
			name:  "unknown var left as-is",
			input: `{"key": "$MISSING"}`,
			env:   map[string]string{},
			want:  `{"key": "$MISSING"}`,
		},
		{
			name:  "underscore in var name",
			input: "key=$API_KEY",
			env:   map[string]string{"API_KEY": "secret"},
			want:  "key=secret",
		},
		{
			name:  "dollar-digit not substituted",
			input: "$123abc",
			env:   map[string]string{"123abc": "x"},
			want:  "$123abc",
		},
		{
			name:  "lone dollar not substituted",
			input: "price: $",
			env:   map[string]string{},
			want:  "price: $",
		},
		{
			name:  "no dollar sign — unchanged",
			input: "no vars here",
			env:   map[string]string{"FOO": "bar"},
			want:  "no vars here",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parser.SubstituteJetBrains(tt.input, tt.env)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSubstituteAll(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{
			name:  "curly brace syntax resolved",
			input: "Bearer {{TOKEN}}",
			env:   map[string]string{"TOKEN": "abc"},
			want:  "Bearer abc",
		},
		{
			name:  "dollar syntax resolved",
			input: "Bearer $TOKEN",
			env:   map[string]string{"TOKEN": "abc"},
			want:  "Bearer abc",
		},
		{
			name:  "both syntaxes in one string",
			input: "{{HOST}}/$PATH",
			env:   map[string]string{"HOST": "https://api.example.com", "PATH": "users"},
			want:  "https://api.example.com/users",
		},
		{
			name:  "unknown vars left as-is",
			input: "{{MISSING}} and $ALSO_MISSING",
			env:   map[string]string{},
			want:  "{{MISSING}} and $ALSO_MISSING",
		},
		{
			name:    "circular reference in curly brace pass returns error",
			input:   "{{FOO}}",
			env:     map[string]string{"FOO": "{{BAR}}", "BAR": "{{FOO}}"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.SubstituteAll(tt.input, tt.env)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMergeEnv(t *testing.T) {
	base := map[string]string{"A": "base-a", "B": "base-b"}
	override := map[string]string{"B": "override-b", "C": "new-c"}

	got := parser.MergeEnv(base, override)

	if got["A"] != "base-a" {
		t.Errorf("A: got %q, want %q", got["A"], "base-a")
	}
	if got["B"] != "override-b" {
		t.Errorf("B: got %q (override should win)", got["B"])
	}
	if got["C"] != "new-c" {
		t.Errorf("C: got %q, want %q", got["C"], "new-c")
	}
}
