package parser

import (
	"fmt"
	"strings"
)

// ParseEnv parses the contents of a .env file and returns a map of key→value.
// Lines starting with # are comments. Empty lines are ignored.
// Values may be optionally quoted with double quotes.
func ParseEnv(content string) (map[string]string, error) {
	env := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue // skip malformed lines silently
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		// Strip surrounding double quotes
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		env[k] = v
	}
	return env, nil
}

// MergeEnv merges override into base, with override values taking precedence.
// Returns a new map; base and override are not modified.
func MergeEnv(base, override map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range override {
		merged[k] = v
	}
	return merged
}

// Substitute replaces all {{VAR}} placeholders in s with values from env.
// Unresolved variables are left as-is ({{VAR}}).
// Returns an error if circular references are detected.
func Substitute(s string, env map[string]string) (string, error) {
	return substitute(s, env, nil)
}

func substitute(s string, env map[string]string, visited map[string]bool) (string, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}

	var result strings.Builder
	remaining := s
	for {
		start := strings.Index(remaining, "{{")
		if start < 0 {
			result.WriteString(remaining)
			break
		}
		end := strings.Index(remaining[start:], "}}")
		if end < 0 {
			result.WriteString(remaining)
			break
		}
		end += start // absolute index of "}}"

		result.WriteString(remaining[:start])
		varName := remaining[start+2 : end]
		remaining = remaining[end+2:]

		val, ok := env[varName]
		if !ok {
			// Unresolved — leave as-is
			result.WriteString("{{")
			result.WriteString(varName)
			result.WriteString("}}")
			continue
		}

		// Circular reference detection
		if visited == nil {
			visited = make(map[string]bool)
		}
		if visited[varName] {
			return "", fmt.Errorf("circular reference detected for variable %q", varName)
		}
		visited[varName] = true

		// Recursively substitute the value in case it also contains {{VAR}}
		expanded, err := substitute(val, env, visited)
		if err != nil {
			return "", err
		}
		delete(visited, varName)

		result.WriteString(expanded)
	}
	return result.String(), nil
}
