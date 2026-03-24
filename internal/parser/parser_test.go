package parser_test

import (
	"strings"
	"testing"

	"github.com/elonnzhang/rest-cli/internal/parser"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []parser.Request
		wantErr bool
	}{
		{
			name:  "single GET no headers no body",
			input: "GET https://example.com/api\n",
			want: []parser.Request{
				{Method: "GET", URL: "https://example.com/api"},
			},
		},
		{
			name: "single POST with headers and body",
			input: "POST https://example.com/api\n" +
				"Content-Type: application/json\n" +
				"Authorization: Bearer token123\n" +
				"\n" +
				`{"key": "value"}` + "\n",
			want: []parser.Request{
				{
					Method: "POST",
					URL:    "https://example.com/api",
					Headers: map[string]string{
						"Content-Type":  "application/json",
						"Authorization": "Bearer token123",
					},
					Body: `{"key": "value"}`,
				},
			},
		},
		{
			name: "named request",
			input: "### Login\n" +
				"POST https://example.com/login\n" +
				"\n" +
				`{"user": "test"}` + "\n",
			want: []parser.Request{
				{
					Name:   "Login",
					Method: "POST",
					URL:    "https://example.com/login",
					Body:   `{"user": "test"}`,
				},
			},
		},
		{
			name: "multiple requests separated by ###",
			input: "### First\n" +
				"GET https://example.com/first\n" +
				"\n" +
				"###\n" +
				"### Second\n" +
				"POST https://example.com/second\n" +
				"\n" +
				`{"x": 1}` + "\n",
			want: []parser.Request{
				{Name: "First", Method: "GET", URL: "https://example.com/first"},
				{Name: "Second", Method: "POST", URL: "https://example.com/second", Body: `{"x": 1}`},
			},
		},
		{
			name: "blank line inside JSON body is NOT a separator",
			input: "POST https://example.com/api\n" +
				"Content-Type: application/json\n" +
				"\n" +
				"{\n" +
				"  \"a\": 1,\n" +
				"\n" +
				"  \"b\": 2\n" +
				"}\n",
			want: []parser.Request{
				{
					Method:  "POST",
					URL:     "https://example.com/api",
					Headers: map[string]string{"Content-Type": "application/json"},
					Body:    "{\n  \"a\": 1,\n\n  \"b\": 2\n}",
				},
			},
		},
		{
			name: "HTTP version suffix stripped",
			input: "GET https://example.com/ HTTP/1.1\n",
			want: []parser.Request{
				{Method: "GET", URL: "https://example.com/"},
			},
		},
		{
			name:  "comment lines ignored",
			input: "# this is a comment\nGET https://example.com/\n# another comment\n",
			want: []parser.Request{
				{Method: "GET", URL: "https://example.com/"},
			},
		},
		{
			name:  "empty file returns no requests",
			input: "",
			want:  []parser.Request{},
		},
		{
			name:  "file with only comments returns no requests",
			input: "# just a comment\n# another\n",
			want:  []parser.Request{},
		},
		{
			name:    "malformed header missing colon returns error",
			input:   "GET https://example.com/\nBadHeader\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.Parse(strings.NewReader(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d requests, want %d\ngot:  %+v\nwant: %+v", len(got), len(tt.want), got, tt.want)
			}
			for i, w := range tt.want {
				g := got[i]
				if g.Name != w.Name {
					t.Errorf("[%d] Name: got %q, want %q", i, g.Name, w.Name)
				}
				if g.Method != w.Method {
					t.Errorf("[%d] Method: got %q, want %q", i, g.Method, w.Method)
				}
				if g.URL != w.URL {
					t.Errorf("[%d] URL: got %q, want %q", i, g.URL, w.URL)
				}
				if g.Body != w.Body {
					t.Errorf("[%d] Body: got %q, want %q", i, g.Body, w.Body)
				}
				for k, wv := range w.Headers {
					gv, ok := g.Headers[k]
					if !ok {
						t.Errorf("[%d] missing header %q", i, k)
					} else if gv != wv {
						t.Errorf("[%d] header %q: got %q, want %q", i, k, gv, wv)
					}
				}
			}
		})
	}
}
