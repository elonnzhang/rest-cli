package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/elonnzhang/rest-cli/internal/parser"
)

const maxResponseBytes = 100 * 1024 * 1024 // 100 MB

// Response holds the result of executing an HTTP request.
type Response struct {
	StatusCode int
	Status     string
	Headers    map[string][]string
	Body       string
	Truncated  bool
	Duration   time.Duration
}

// Execute sends a parsed HTTP request and returns the response.
// It respects ctx cancellation (use context.WithTimeout for timeout,
// or call cancel() to abort an in-flight request from the TUI).
// HTTP 4xx/5xx responses are NOT errors — they are valid responses.
// Only network failures, timeouts, and context cancellations are errors.
func Execute(ctx context.Context, req parser.Request) (*Response, error) {
	var bodyReader io.Reader
	if req.Body != "" {
		bodyReader = strings.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	start := time.Now()
	httpResp, err := http.DefaultClient.Do(httpReq)
	duration := time.Since(start)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	// Read up to maxResponseBytes
	limited := io.LimitReader(httpResp.Body, maxResponseBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	truncated := false
	body := string(raw)
	if int64(len(raw)) > maxResponseBytes {
		body = body[:maxResponseBytes]
		truncated = true
	}

	return &Response{
		StatusCode: httpResp.StatusCode,
		Status:     httpResp.Status,
		Headers:    map[string][]string(httpResp.Header),
		Body:       body,
		Truncated:  truncated,
		Duration:   duration,
	}, nil
}
