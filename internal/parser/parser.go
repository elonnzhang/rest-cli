package parser

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Request represents a single HTTP request parsed from a .http file.
type Request struct {
	Name    string
	Method  string
	URL     string
	Headers map[string]string
	Body    string
	Vars    map[string]string // @key= value declarations scoped to this request
	Line    int               // 1-based line number of the ### separator (or METHOD line if no separator)
}

// Parse reads a .http file and returns all requests found in it.
// Multiple requests are separated by lines starting with "###".
// Comment lines (starting with "#" but not "###") are ignored.
// Returns an error if a header line is malformed (missing colon).
func Parse(r io.Reader) ([]Request, error) {
	scanner := bufio.NewScanner(r)

	var requests []Request
	var current *requestBuilder
	var lineNum int
	var fileVars map[string]string // @var declarations before any request block

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// "###" separator (with optional name after it)
		if strings.HasPrefix(line, "###") {
			if current != nil {
				req, err := current.build()
				if err != nil {
					return nil, err
				}
				if req.Method != "" {
					requests = append(requests, req)
				}
			}
			current = &requestBuilder{line: lineNum}
			name := strings.TrimSpace(strings.TrimPrefix(line, "###"))
			if name != "" {
				current.name = name
			}
			continue
		}

		// Skip standalone comment lines (# but not ###)
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Skip JetBrains-specific metadata lines:
		// //TIP ... — tooltip annotations
		// // ...    — double-slash comments
		// >> / >>!   — response-save directives
		if strings.HasPrefix(line, "//") ||
			strings.HasPrefix(line, ">>") {
			continue
		}

		// @key= value — variable declarations (file-scoped).
		// All @var declarations anywhere in the file are treated as file-level
		// and propagated to every request. @no-redirect etc. (no "=") are skipped.
		if strings.HasPrefix(line, "@") {
			if k, v, ok := parseAtVar(line); ok {
				if fileVars == nil {
					fileVars = make(map[string]string)
				}
				fileVars[k] = v
			}
			continue
		}

		if current == nil {
			current = &requestBuilder{line: lineNum}
		}
		if err := current.feed(line); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Flush last request
	if current != nil {
		req, err := current.build()
		if err != nil {
			return nil, err
		}
		if req.Method != "" {
			requests = append(requests, req)
		}
	}

	if requests == nil {
		requests = []Request{}
	}

	// Attach file-level vars to every request.
	for i := range requests {
		requests[i].Vars = fileVars
	}

	return requests, nil
}

// requestBuilder accumulates lines for a single request.
type requestBuilder struct {
	name    string
	method  string
	url     string
	headers map[string]string
	body    strings.Builder
	line    int // 1-based line of ### separator or METHOD line

	// state machine
	seenRequestLine bool
	seenBlankLine   bool
	hasBody         bool
}

// feed processes one line of input.
func (b *requestBuilder) feed(line string) error {
	if !b.seenRequestLine {
		// First non-comment non-separator line is the request line
		if line == "" {
			return nil // blank lines before request line are ignored
		}
		method, url, err := parseRequestLine(line)
		if err != nil {
			return err
		}
		b.method = method
		b.url = url
		b.seenRequestLine = true
		return nil
	}

	if !b.seenBlankLine {
		if line == "" {
			b.seenBlankLine = true
			return nil
		}
		// Multi-line URL continuation: indented line starting with ? or &
		// e.g. "    ?param=value" after the request line
		trimmed := strings.TrimSpace(line)
		if b.headers == nil && (strings.HasPrefix(trimmed, "?") || strings.HasPrefix(trimmed, "&")) {
			b.url += trimmed
			return nil
		}
		// Header line
		k, v, err := parseHeader(line)
		if err != nil {
			return err
		}
		if b.headers == nil {
			b.headers = make(map[string]string)
		}
		b.headers[k] = v
		return nil
	}

	// Body: everything after the first blank line
	if b.hasBody {
		b.body.WriteByte('\n')
	}
	b.body.WriteString(line)
	b.hasBody = true
	return nil
}

func (b *requestBuilder) build() (Request, error) {
	if !b.seenRequestLine {
		// Empty section (e.g. trailing ### with no request)
		return Request{}, nil
	}
	return Request{
		Name:    b.name,
		Method:  b.method,
		URL:     b.url,
		Headers: b.headers,
		Body:    b.body.String(),
		Line:    b.line,
	}, nil
}

// parseRequestLine parses "METHOD URL [HTTP/version]" and returns method and URL.
func parseRequestLine(line string) (method, url string, err error) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid request line: %q", line)
	}
	method = parts[0]
	url = parts[1]
	// Strip optional HTTP version suffix
	return method, url, nil
}

// parseAtVar parses "@key= value" or "@key = value" into key and value.
// Returns ok=false for lines without "=" (e.g. "@no-redirect").
func parseAtVar(line string) (key, value string, ok bool) {
	rest := strings.TrimPrefix(line, "@")
	idx := strings.IndexByte(rest, '=')
	if idx < 0 {
		return "", "", false
	}
	k := strings.TrimSpace(rest[:idx])
	v := strings.TrimSpace(rest[idx+1:])
	if k == "" {
		return "", "", false
	}
	return k, v, true
}

// parseHeader parses "Key: Value" and returns key and value.
func parseHeader(line string) (key, value string, err error) {
	k, v, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", fmt.Errorf("malformed header (missing colon): %q", line)
	}
	return strings.TrimSpace(k), strings.TrimSpace(v), nil
}
