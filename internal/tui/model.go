package tui

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/elonnzhang/rest-cli/internal/client"
	"github.com/elonnzhang/rest-cli/internal/parser"
)

// itemKind distinguishes between file-level and request-level tree items.
type itemKind int

const (
	kindFile    itemKind = iota
	kindRequest          // an individual request within a .http file
)

// treeItem is one visible row in the two-level sidebar tree.
type treeItem struct {
	kind     itemKind
	filePath string // absolute path of the .http file
	reqIdx   int    // request index within the file (kindRequest only)
	label    string // display text (pre-computed for requests)
}

// dragKind identifies which pane separator is being dragged.
type dragKind int

const (
	dragNone    dragKind = iota
	dragSidebar          // dragging the │ divider → adjusts paneW
	dragReqSep           // dragging the ─ separator between req/resp → adjusts reqPaneH
)

// ResponseMsg is sent by the executeRequest Cmd when a response arrives.
type ResponseMsg struct {
	Key      string // "path:idx" key identifying which request completed
	Req      parser.Request
	Response *client.Response
	Err      error
}

// FilesDiscoveredMsg is sent when the file-walk Cmd completes.
type FilesDiscoveredMsg struct {
	Files []string
}

// editorClosedMsg is sent after the external editor process exits.
type editorClosedMsg struct {
	path string
	err  error
}

// Config holds the startup configuration for the TUI.
type Config struct {
	Files   []string
	Dir     string // root directory to search; defaults to CWD
	Env     map[string]string
	History *client.History
}

// Model is the single root Bubble Tea model. It holds ALL state.
// All async work goes through tea.Cmd / tea.Msg — no raw goroutines.
type Model struct {
	// file browser
	files         []string
	cursor        int // index into flatTree()
	sidebarScroll int // number of tree rows scrolled off the top
	searching     bool
	query         textinput.Model

	// tree state
	expanded map[string]bool             // filePath → is expanded
	fileReqs map[string][]parser.Request // filePath → cached requests

	// selected request (shown in right pane)
	selected    *parser.Request
	selectedErr error

	// per-request execution state (keyed by "path:idx")
	inFlight  map[string]bool
	cancels   map[string]context.CancelFunc
	responses map[string]*client.Response
	errors    map[string]error

	// history
	history *client.History

	// layout — mutable pane sizes
	width    int
	height   int
	paneW    int      // sidebar width (default 30, resizable)
	reqPaneH int      // request pane height (default 9, resizable)
	dragging dragKind // which separator is being dragged (mouse)

	// right-pane view state
	selectedFile   string // non-empty when a Level-1 file is focused → show request list
	selectedReqKey string // "path:idx" key used to detect request navigation

	// env
	env map[string]string

	// config
	dir string
}

const defaultPaneW = 30
const defaultReqPaneH = 9

// New creates a Model with the given config.
func New(cfg Config) Model {
	ti := textinput.New()
	ti.Placeholder = "fuzzy search..."
	ti.CharLimit = 100

	h := cfg.History
	if h == nil {
		h = client.NewHistory(100)
	}

	dir := cfg.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	return Model{
		files:     cfg.Files,
		query:     ti,
		history:   h,
		env:       cfg.Env,
		dir:       dir,
		expanded:  make(map[string]bool),
		fileReqs:  make(map[string][]parser.Request),
		inFlight:  make(map[string]bool),
		cancels:   make(map[string]context.CancelFunc),
		responses: make(map[string]*client.Response),
		errors:    make(map[string]error),
		paneW:     defaultPaneW,
		reqPaneH:  defaultReqPaneH,
	}
}

// Init kicks off file discovery if no files were pre-loaded.
func (m Model) Init() tea.Cmd {
	if len(m.files) == 0 && m.dir != "" {
		return discoverFiles(m.dir)
	}
	return nil
}

// Update handles all messages. Pure function — side effects go into Cmds.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampPaneSizes()
		return m, nil

	case FilesDiscoveredMsg:
		m.files = msg.Files
		if len(m.files) == 0 {
			m.selectedErr = fmt.Errorf("no .http files found in %s. Use --dir to specify a directory", m.dir)
		} else {
			m.cursor = 0
			m.loadSelectedRequest()
		}
		return m, nil

	case ResponseMsg:
		delete(m.inFlight, msg.Key)
		delete(m.cancels, msg.Key)
		if msg.Err != nil {
			m.errors[msg.Key] = msg.Err
			delete(m.responses, msg.Key)
		} else {
			m.responses[msg.Key] = msg.Response
			delete(m.errors, msg.Key)
			if msg.Response != nil {
				m.history.Add(client.HistoryEntry{
					Method:     msg.Req.Method,
					URL:        msg.Req.URL,
					StatusCode: msg.Response.StatusCode,
					Duration:   msg.Response.Duration,
				})
			}
		}
		return m, nil

	case editorClosedMsg:
		// Reload the file's cached requests after editing.
		delete(m.fileReqs, msg.path)
		if m.expanded[msg.path] {
			reqs, _ := parseFile(msg.path)
			m.fileReqs[msg.path] = reqs
		}
		m.loadSelectedRequest()
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.searching {
		var cmd tea.Cmd
		m.query, cmd = m.query.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C: cancel the selected request if in-flight, otherwise quit
	if msg.Type == tea.KeyCtrlC {
		if m.inFlight[m.selectedReqKey] {
			if cancel := m.cancels[m.selectedReqKey]; cancel != nil {
				cancel()
			}
			delete(m.inFlight, m.selectedReqKey)
			delete(m.cancels, m.selectedReqKey)
			m.errors[m.selectedReqKey] = fmt.Errorf("request cancelled")
			return m, nil
		}
		return m, tea.Quit
	}

	// When searching, ↑/↓ navigate filtered results; everything else goes to the textinput.
	if m.searching {
		switch msg.Type {
		case tea.KeyEsc:
			m.searching = false
			m.query.Blur()
			m.query.SetValue("")
			m.cursor = 0
			m.sidebarScroll = 0
			m.loadSelectedRequest()
			return m, nil
		case tea.KeyEnter:
			m.searching = false
			m.query.Blur()
			return m.executeSelected()
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
				m.clampScroll()
				m.loadSelectedRequest()
			}
			return m, nil
		case tea.KeyDown:
			if m.cursor < len(m.flatTree())-1 {
				m.cursor++
				m.clampScroll()
				m.loadSelectedRequest()
			}
			return m, nil
		default:
			prev := m.query.Value()
			var cmd tea.Cmd
			m.query, cmd = m.query.Update(msg)
			// Reset cursor when the query text changes so it stays within filtered results.
			if m.query.Value() != prev {
				m.cursor = 0
				m.sidebarScroll = 0
				m.loadSelectedRequest()
			}
			return m, cmd
		}
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m.clampScroll()
			m.loadSelectedRequest()
		}
		return m, nil

	case tea.KeyDown:
		if m.cursor < len(m.flatTree())-1 {
			m.cursor++
			m.clampScroll()
			m.loadSelectedRequest()
		}
		return m, nil

	case tea.KeyEnter:
		items := m.flatTree()
		if len(items) == 0 || m.cursor >= len(items) {
			return m, nil
		}
		item := items[m.cursor]
		if item.kind == kindFile {
			// Level 1: toggle expand/collapse
			m.toggleExpand(item.filePath)
			m.clampScroll()
			return m, nil
		}
		// Level 2: execute this request
		return m.executeSelected()

	case tea.KeyEsc:
		m.searching = false
		m.query.SetValue("")
		return m, nil

	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "/":
			m.searching = true
			m.query.Focus()
			// Parse all files eagerly so search can match request labels.
			for _, f := range m.files {
				if _, ok := m.fileReqs[f]; !ok {
					if reqs, err := parseFile(f); err == nil {
						m.fileReqs[f] = reqs
					}
				}
			}
			return m, nil
		case "q":
			return m, tea.Quit
		case "r":
			m.expanded = make(map[string]bool)
			m.fileReqs = make(map[string][]parser.Request)
			return m, discoverFiles(m.dir)
		case "e":
			return m.openEditor()

		// Pane resize keys:
		// [ / ] → sidebar narrower / wider
		// { / } → request pane shorter / taller
		case "[":
			if m.paneW > 15 {
				m.paneW--
				m.clampScroll()
			}
			return m, nil
		case "]":
			if m.paneW < m.width-20 {
				m.paneW++
				m.clampScroll()
			}
			return m, nil
		case "{":
			if m.reqPaneH > 3 {
				m.reqPaneH--
			}
			return m, nil
		case "}":
			if m.reqPaneH < m.height-8 {
				m.reqPaneH++
			}
			return m, nil
		}
	}

	return m, nil
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.MouseLeft:
		// In Bubble Tea, holding a button and moving fires repeated MouseLeft
		// events (not MouseMotion). Apply drag update if already dragging.
		if m.dragging != dragNone {
			return m.applyDrag(msg.X, msg.Y)
		}
		// Start drag if clicking on the │ divider or ─ separator
		if msg.X == m.paneW {
			m.dragging = dragSidebar
			return m, nil
		}
		if msg.X > m.paneW && msg.Y == m.reqPaneH {
			m.dragging = dragReqSep
			return m, nil
		}
		// Click in sidebar
		if msg.X < m.paneW && msg.Y >= 3 && msg.Y < m.height-1 {
			itemIdx := msg.Y - 3 + m.sidebarScroll
			items := m.flatTree()
			if itemIdx >= 0 && itemIdx < len(items) {
				if m.cursor == itemIdx {
					// Second click on same item: activate
					item := items[itemIdx]
					if item.kind == kindFile {
						m.toggleExpand(item.filePath)
						m.clampScroll()
						return m, nil
					}
					return m.executeSelected()
				}
				m.cursor = itemIdx
				m.clampScroll()
				m.loadSelectedRequest()
			}
		}

	case tea.MouseMotion:
		// Fallback: also handle motion events (fires when moving without button)
		if m.dragging != dragNone {
			return m.applyDrag(msg.X, msg.Y)
		}

	case tea.MouseRelease:
		m.dragging = dragNone

	case tea.MouseWheelUp:
		if msg.X < m.paneW && m.sidebarScroll > 0 {
			m.sidebarScroll--
		}

	case tea.MouseWheelDown:
		if msg.X < m.paneW {
			items := m.flatTree()
			listH := m.sidebarListH()
			if max := len(items) - listH; max > 0 && m.sidebarScroll < max {
				m.sidebarScroll++
			}
		}
	}
	return m, nil
}

// flatTree returns the currently visible tree items (files + expanded requests),
// filtered by the search query against request labels (not filenames).
func (m Model) flatTree() []treeItem {
	query := m.query.Value()
	var items []treeItem

	for _, f := range m.files {
		reqs := m.fileReqs[f]

		if query == "" {
			items = append(items, treeItem{kind: kindFile, filePath: f})
			if m.expanded[f] {
				for i, req := range reqs {
					items = append(items, treeItem{
						kind:     kindRequest,
						filePath: f,
						reqIdx:   i,
						label:    requestLabel(req),
					})
				}
			}
			continue
		}

		// Search mode: match query against request labels.
		if len(reqs) == 0 {
			continue
		}
		labels := make([]string, len(reqs))
		for i, req := range reqs {
			labels[i] = requestLabel(req)
		}
		hits := FuzzyMatchIndices(query, labels)
		if len(hits) == 0 {
			continue
		}
		items = append(items, treeItem{kind: kindFile, filePath: f})
		for _, idx := range hits {
			items = append(items, treeItem{
				kind:     kindRequest,
				filePath: f,
				reqIdx:   idx,
				label:    labels[idx],
			})
		}
	}
	return items
}

// requestLabel returns the display label for a request.
func requestLabel(req parser.Request) string {
	if req.Name != "" {
		return req.Name
	}
	return req.Method + " " + shortenURL(req.URL)
}

// toggleExpand expands or collapses a file. Parses and caches requests on first expand.
func (m *Model) toggleExpand(path string) {
	if m.expanded[path] {
		delete(m.expanded, path)
		return
	}
	if _, ok := m.fileReqs[path]; !ok {
		reqs, err := parseFile(path)
		if err != nil {
			m.selectedErr = err
			return
		}
		m.fileReqs[path] = reqs
	}
	m.expanded[path] = true
}

// loadSelectedRequest updates m.selected / m.selectedFile based on the current cursor.
//
//   - Level 1 (file row): sets m.selectedFile so the right pane shows the request list.
//   - Level 2 (request row): sets m.selected with the specific request.
//     Cached responses are preserved per-key (not cleared on navigation).
func (m *Model) loadSelectedRequest() {
	items := m.flatTree()
	if len(items) == 0 || m.cursor >= len(items) {
		return
	}
	item := items[m.cursor]

	// Ensure the file is parsed and cached.
	if _, ok := m.fileReqs[item.filePath]; !ok {
		reqs, err := parseFile(item.filePath)
		if err != nil {
			m.selectedErr = err
			m.selected = nil
			m.selectedFile = ""
			return
		}
		m.fileReqs[item.filePath] = reqs
	}

	if item.kind == kindFile {
		// Level 1: show the file's request list in the right pane.
		m.selectedFile = item.filePath
		m.selected = nil
		m.selectedErr = nil
		return
	}

	// Level 2: show the specific request detail.
	reqs := m.fileReqs[item.filePath]
	if item.reqIdx >= len(reqs) {
		return
	}
	newKey := fmt.Sprintf("%s:%d", item.filePath, item.reqIdx)
	m.selectedReqKey = newKey
	req := reqs[item.reqIdx]
	m.selected = &req
	m.selectedFile = ""
	m.selectedErr = nil
}

// executeSelected fires an HTTP request for the currently selected tree item.
func (m Model) executeSelected() (tea.Model, tea.Cmd) {
	items := m.flatTree()
	if len(items) == 0 || m.cursor >= len(items) {
		return m, nil
	}
	item := items[m.cursor]

	if _, ok := m.fileReqs[item.filePath]; !ok {
		reqs, err := parseFile(item.filePath)
		if err != nil {
			m.selectedErr = err
			return m, nil
		}
		m.fileReqs[item.filePath] = reqs
	}

	reqs := m.fileReqs[item.filePath]
	if len(reqs) == 0 {
		return m, nil
	}

	idx := 0
	if item.kind == kindRequest {
		idx = item.reqIdx
	}
	req := reqs[idx]
	m.selected = &req
	m.selectedErr = nil

	// Merge global env with request-local @var declarations (request vars win).
	subEnv := make(map[string]string, len(m.env)+len(req.Vars))
	for k, v := range m.env {
		subEnv[k] = v
	}
	for k, v := range req.Vars {
		subEnv[k] = v
	}
	// Substitute env vars at execution time (not parse time)
	req.URL, _ = parser.SubstituteAll(req.URL, subEnv)
	for k, v := range req.Headers {
		req.Headers[k], _ = parser.SubstituteAll(v, subEnv)
	}
	req.Body, _ = parser.SubstituteAll(req.Body, subEnv)

	key := fmt.Sprintf("%s:%d", item.filePath, idx)
	m.selectedReqKey = key
	// Cancel any existing in-flight for this key before re-running
	if cancel, ok := m.cancels[key]; ok {
		cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.inFlight[key] = true
	m.cancels[key] = cancel

	return m, executeRequest(key, req, ctx)
}

// openEditor opens the currently selected .http file in $EDITOR at the request's line.
func (m Model) openEditor() (tea.Model, tea.Cmd) {
	items := m.flatTree()
	if len(items) == 0 || m.cursor >= len(items) {
		return m, nil
	}
	item := items[m.cursor]
	line := 1
	if reqs, ok := m.fileReqs[item.filePath]; ok {
		idx := 0
		if item.kind == kindRequest {
			idx = item.reqIdx
		}
		if idx < len(reqs) && reqs[idx].Line > 0 {
			line = reqs[idx].Line
		}
	}
	return m, editFile(item.filePath, line)
}

// sidebarListH returns the number of visible rows available for tree items.
func (m Model) sidebarListH() int {
	h := m.height - 5
	if h < 1 {
		h = 1
	}
	return h
}

// clampScroll adjusts sidebarScroll so the cursor is always visible.
func (m *Model) clampScroll() {
	listH := m.sidebarListH()
	if m.cursor < m.sidebarScroll {
		m.sidebarScroll = m.cursor
	}
	if m.cursor >= m.sidebarScroll+listH {
		m.sidebarScroll = m.cursor - listH + 1
	}
	if m.sidebarScroll < 0 {
		m.sidebarScroll = 0
	}
}

// applyDrag updates pane sizes during a mouse drag operation.
func (m Model) applyDrag(x, y int) (tea.Model, tea.Cmd) {
	switch m.dragging {
	case dragSidebar:
		newW := x
		if newW < 15 {
			newW = 15
		}
		if m.width > 20 && newW > m.width-20 {
			newW = m.width - 20
		}
		m.paneW = newW
		m.clampScroll()
	case dragReqSep:
		newH := y
		if newH < 3 {
			newH = 3
		}
		if m.height > 8 && newH > m.height-8 {
			newH = m.height - 8
		}
		m.reqPaneH = newH
	}
	return m, nil
}

// clampPaneSizes ensures pane dimensions stay within valid bounds.
func (m *Model) clampPaneSizes() {
	if m.paneW < 15 {
		m.paneW = 15
	}
	if m.width > 20 && m.paneW > m.width-20 {
		m.paneW = m.width - 20
	}
	if m.reqPaneH < 3 {
		m.reqPaneH = 3
	}
	if m.height > 8 && m.reqPaneH > m.height-8 {
		m.reqPaneH = m.height - 8
	}
}

// --- Cmds ---

// discoverFiles walks dir recursively and collects all *.http files.
func discoverFiles(dir string) tea.Cmd {
	return func() tea.Msg {
		var files []string
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && strings.HasSuffix(path, ".http") {
				abs, aerr := filepath.Abs(path)
				if aerr == nil {
					files = append(files, abs)
				} else {
					files = append(files, path)
				}
			}
			return nil
		})
		return FilesDiscoveredMsg{Files: files}
	}
}

// executeRequest sends the HTTP request and returns a Cmd that produces a ResponseMsg.
func executeRequest(key string, req parser.Request, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.Execute(ctx, req)
		return ResponseMsg{Key: key, Req: req, Response: resp, Err: err}
	}
}

// editFile opens a .http file in $EDITOR at the given line (falls back to vi).
// Most terminal editors (vi, vim, nvim, nano, emacs) accept +N to jump to line N.
func editFile(path string, line int) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	args := []string{}
	if line > 1 {
		args = append(args, fmt.Sprintf("+%d", line))
	}
	args = append(args, path)
	c := exec.Command(editor, args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorClosedMsg{path: path, err: err}
	})
}

// --- Helpers ---

func parseFile(path string) ([]parser.Request, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parser.Parse(f)
}

// --- Accessors (used by tests) ---

func (m Model) SelectedFileIndex() int         { return m.cursor }
func (m Model) Searching() bool                { return m.searching }
func (m Model) InFlight() bool                 { return m.inFlight[m.selectedReqKey] }
func (m Model) LastResponse() *client.Response { return m.responses[m.selectedReqKey] }
func (m Model) LastErr() error                 { return m.errors[m.selectedReqKey] }
func (m Model) Files() []string                { return m.files }
func (m Model) Size() (int, int)               { return m.width, m.height }

// WithInFlight returns a copy with inFlight set — used in tests.
func (m Model) WithInFlight(v bool) Model {
	if m.inFlight == nil {
		m.inFlight = make(map[string]bool)
	}
	if v {
		m.inFlight[m.selectedReqKey] = true
	} else {
		delete(m.inFlight, m.selectedReqKey)
	}
	return m
}

// WithCancelFunc returns a copy with the cancel func set — used in tests.
func (m Model) WithCancelFunc(f context.CancelFunc) Model {
	if m.cancels == nil {
		m.cancels = make(map[string]context.CancelFunc)
	}
	m.cancels[m.selectedReqKey] = f
	return m
}
