package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCmd(t *testing.T) {
	// Shared test server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hello":
			w.WriteHeader(200)
			fmt.Fprint(w, `{"hello":"world"}`)
		case "/auth":
			if r.Header.Get("Authorization") != "Bearer testtoken" {
				w.WriteHeader(401)
				return
			}
			w.WriteHeader(200)
			fmt.Fprint(w, `{"authed":true}`)
		case "/echo":
			w.WriteHeader(201)
			fmt.Fprintf(w, `{"method":"%s"}`, r.Method)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	writeHTTP := func(t *testing.T, content string) string {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.http")
		// Replace placeholder with real server URL
		content = strings.ReplaceAll(content, "{{SERVER}}", srv.URL)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	tests := []struct {
		name       string
		httpFile   string
		args       []string
		wantStdout string
		wantStderr string
		wantExit   int
	}{
		{
			name:       "run first request",
			httpFile:   "GET {{SERVER}}/hello\n",
			wantStdout: `{"hello":"world"}`,
			wantStderr: "200 OK",
			wantExit:   0,
		},
		{
			name:       "run named request",
			httpFile:   "### Ping\nGET {{SERVER}}/hello\n\n###\n### Echo\nPOST {{SERVER}}/echo\n",
			args:       []string{"-n", "Echo"},
			wantStdout: `{"method":"POST"}`,
			wantExit:   0,
		},
		{
			name:     "named request not found exits 1",
			httpFile: "GET {{SERVER}}/hello\n",
			args:     []string{"-n", "Missing"},
			wantExit: 1,
		},
		{
			name:     "missing file exits 1",
			httpFile: "", // signals: don't create file
			wantExit: 1,
		},
		{
			name: "env var substitution from .env file",
			httpFile: "GET {{SERVER}}/auth\n" +
				"Authorization: Bearer {{TOKEN}}\n",
			wantStdout: `{"authed":true}`,
			wantExit:   0,
		},
		{
			name:     "no requests in file exits 1",
			httpFile: "# just a comment\n",
			wantExit: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			var fileArgs []string
			if tt.httpFile == "" {
				fileArgs = []string{"/nonexistent/file.http"}
			} else {
				path := writeHTTP(t, tt.httpFile)
				// For the env test, create a .env file alongside
				if strings.Contains(tt.name, "env var") {
					envPath := filepath.Join(filepath.Dir(path), ".env")
					os.WriteFile(envPath, []byte("TOKEN=testtoken\n"), 0o644)
				}
				fileArgs = []string{path}
			}

			// flags must come before the positional file arg (Go flag package convention)
			allArgs := append([]string{"run"}, tt.args...)
			allArgs = append(allArgs, fileArgs...)

			exitCode := run(allArgs, &stdout, &stderr)
			if exitCode != tt.wantExit {
				t.Errorf("exit code: got %d, want %d\nstdout: %s\nstderr: %s",
					exitCode, tt.wantExit, stdout.String(), stderr.String())
			}
			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout: got %q, want to contain %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr: got %q, want to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
