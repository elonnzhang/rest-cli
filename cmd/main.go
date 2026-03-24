package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elonnzhang/rest-cli/internal/client"
	"github.com/elonnzhang/rest-cli/internal/parser"
	"github.com/elonnzhang/rest-cli/internal/tui"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the testable entrypoint. Returns exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "run" {
		return runCmd(args[1:], stdout, stderr)
	}
	return runTUI(args, stderr)
}

func runCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		name    = fs.String("n", "", "request name to run (default: first request)")
		envFile = fs.String("env", "", "additional .env file (overrides .env.local and .env)")
		verbose = fs.Bool("v", false, "print response headers to stderr")
		save    = fs.String("save", "", "write response body to file instead of stdout")
		timeout = fs.Duration("timeout", 30*time.Second, "request timeout")
	)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "usage: rest-cli run [flags] <file.http>")
		return 1
	}
	httpFile := fs.Arg(0)

	// --- Parse .http file ---
	f, err := os.Open(httpFile)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	defer f.Close()

	requests, err := parser.Parse(f)
	if err != nil {
		fmt.Fprintf(stderr, "parse error: %v\n", err)
		return 1
	}
	if len(requests) == 0 {
		fmt.Fprintln(stderr, "error: no requests found in file")
		return 1
	}

	// --- Select request ---
	var req parser.Request
	if *name != "" {
		found := false
		for _, r := range requests {
			if r.Name == *name {
				req = r
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(stderr, "error: request %q not found in %s\n", *name, httpFile)
			return 1
		}
	} else {
		req = requests[0]
	}

	// --- Load env files ---
	env := loadEnvChain(filepath.Dir(httpFile), *envFile, stderr)

	// --- Substitute variables ---
	req.URL, err = parser.Substitute(req.URL, env)
	if err != nil {
		fmt.Fprintf(stderr, "env substitution error: %v\n", err)
		return 1
	}
	for k, v := range req.Headers {
		subbed, serr := parser.Substitute(v, env)
		if serr != nil {
			fmt.Fprintf(stderr, "env substitution error in header %q: %v\n", k, serr)
			return 1
		}
		req.Headers[k] = subbed
	}
	if req.Body != "" {
		req.Body, err = parser.Substitute(req.Body, env)
		if err != nil {
			fmt.Fprintf(stderr, "env substitution error in body: %v\n", err)
			return 1
		}
	}

	// --- Execute ---
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	resp, err := client.Execute(ctx, req)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	// --- Output ---
	// Status line → stderr
	fmt.Fprintf(stderr, "%s  (%s)\n", resp.Status, resp.Duration.Round(time.Millisecond))

	// Verbose: headers → stderr
	if *verbose {
		for k, vals := range resp.Headers {
			for _, v := range vals {
				fmt.Fprintf(stderr, "%s: %s\n", k, v)
			}
		}
		fmt.Fprintln(stderr)
	}

	// Body → stdout or file
	body := resp.Body
	if resp.Truncated {
		fmt.Fprintf(stderr, "[Truncated — response >100MB. Use: rest-cli run --save <file>]\n")
	}

	if *save != "" {
		if werr := os.WriteFile(*save, []byte(body), 0o644); werr != nil {
			fmt.Fprintf(stderr, "error writing to %s: %v\n", *save, werr)
			return 1
		}
		fmt.Fprintf(stderr, "body saved to %s\n", *save)
	} else {
		fmt.Fprint(stdout, body)
		if body != "" && !strings.HasSuffix(body, "\n") {
			fmt.Fprintln(stdout) // ensure trailing newline
		}
	}

	return 0
}

// runTUI launches the interactive Bubble Tea TUI.
func runTUI(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dirFlag := fs.String("dir", "", "root directory to search for .http files (default: CWD)")
	envFile := fs.String("env", "", "additional .env file")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	cwd, _ := os.Getwd()
	dir := cwd
	if *dirFlag != "" {
		if abs, err := filepath.Abs(*dirFlag); err == nil {
			dir = abs
		} else {
			dir = *dirFlag
		}
	}

	env := loadEnvChain(dir, *envFile, stderr)

	m := tui.New(tui.Config{
		Dir: dir,
		Env: env,
	})

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

// loadEnvChain loads .env, then .env.local, then the explicit --env file,
// each layer overriding the previous. Missing files are silently skipped.
func loadEnvChain(dir, extraEnvFile string, stderr io.Writer) map[string]string {
	env := make(map[string]string)
	for _, name := range []string{".env", ".env.local"} {
		path := filepath.Join(dir, name)
		m := readEnvFile(path)
		env = parser.MergeEnv(env, m)
	}
	if extraEnvFile != "" {
		m := readEnvFile(extraEnvFile)
		env = parser.MergeEnv(env, m)
	}
	return env
}

// readEnvFile reads and parses a .env file. Returns empty map if the file
// doesn't exist or can't be read.
func readEnvFile(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	m, err := parser.ParseEnv(string(data))
	if err != nil {
		return map[string]string{}
	}
	return m
}
