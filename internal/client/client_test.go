package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/elonnzhang/rest-cli/internal/client"
	"github.com/elonnzhang/rest-cli/internal/parser"
)

func TestExecute(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		req        parser.Request
		wantStatus int
		wantBody   string
		wantErr    bool
	}{
		{
			name: "GET 200 OK",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"ok":true}`))
			},
			req:        parser.Request{Method: "GET", URL: "{{SERVER}}/"},
			wantStatus: 200,
			wantBody:   `{"ok":true}`,
		},
		{
			name: "POST with body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"created":true}`))
			},
			req: parser.Request{
				Method: "POST",
				URL:    "{{SERVER}}/",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: `{"name":"test"}`,
			},
			wantStatus: 201,
			wantBody:   `{"created":true}`,
		},
		{
			name: "404 response is not an error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"not found"}`))
			},
			req:        parser.Request{Method: "GET", URL: "{{SERVER}}/missing"},
			wantStatus: 404,
			wantBody:   `{"error":"not found"}`,
		},
		{
			name: "500 response is not an error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"server error"}`))
			},
			req:        parser.Request{Method: "GET", URL: "{{SERVER}}/boom"},
			wantStatus: 500,
			wantBody:   `{"error":"server error"}`,
		},
		{
			name: "headers are sent",
			handler: func(w http.ResponseWriter, r *http.Request) {
				auth := r.Header.Get("Authorization")
				if auth != "Bearer secret" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"auth":true}`))
			},
			req: parser.Request{
				Method:  "GET",
				URL:     "{{SERVER}}/protected",
				Headers: map[string]string{"Authorization": "Bearer secret"},
			},
			wantStatus: 200,
			wantBody:   `{"auth":true}`,
		},
		{
			name: "context cancellation returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(500 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
			},
			req:     parser.Request{Method: "GET", URL: "{{SERVER}}/slow"},
			wantErr: true,
		},
		{
			name: "DELETE request",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			},
			req:        parser.Request{Method: "DELETE", URL: "{{SERVER}}/item/1"},
			wantStatus: 204,
		},
		{
			name: "PUT request with body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"updated":true}`))
			},
			req: parser.Request{
				Method: "PUT",
				URL:    "{{SERVER}}/item/1",
				Body:   `{"name":"updated"}`,
			},
			wantStatus: 200,
			wantBody:   `{"updated":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			// Replace {{SERVER}} placeholder with the test server URL
			req := tt.req
			req.URL = strings.ReplaceAll(req.URL, "{{SERVER}}", srv.URL)

			ctx := context.Background()
			if tt.wantErr {
				// Use a context that is already cancelled
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
			}

			resp, err := client.Execute(ctx, req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status: got %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if tt.wantBody != "" && resp.Body != tt.wantBody {
				t.Errorf("body: got %q, want %q", resp.Body, tt.wantBody)
			}
			if resp.Duration <= 0 {
				t.Error("duration should be positive")
			}
		})
	}
}
