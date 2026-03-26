package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// colours
var (
	cBlue   = lipgloss.Color("#4a9eff")
	cGreen  = lipgloss.Color("#4caf50")
	cOrange = lipgloss.Color("#ff9800")
	cRed    = lipgloss.Color("#f44336")
	cPurple = lipgloss.Color("#9c27b0")
	cGray   = lipgloss.Color("#888888")
	cDim    = lipgloss.Color("#555555")
	cText   = lipgloss.Color("#d0d0d0")
	cMuted  = lipgloss.Color("#999999")
	cSelBg  = lipgloss.Color("#1d3557")
	cBarBg  = lipgloss.Color("#1a1a1a")
	cBorder = lipgloss.Color("#444444")
	cVar    = lipgloss.Color("#e5c07b") // amber — for {{variables}}
)

// line returns a string padded/truncated to exactly w visible columns.
func line(w int, s string) string {
	return lipgloss.NewStyle().Width(w).MaxWidth(w).Render(s)
}

// View renders the complete TUI. Builds left and right panes as line slices,
// then zips them with a │ divider — no lipgloss JoinHorizontal needed.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	rightW := m.width - m.paneW - 1
	contentH := m.height - 1 // reserve bottom line for status bar

	leftLines := m.leftPane(contentH)
	rightLines := m.rightPane(rightW, contentH)

	// Highlight divider when being dragged
	divStyle := lipgloss.NewStyle().Foreground(cBorder)
	if m.dragging == dragSidebar {
		divStyle = lipgloss.NewStyle().Foreground(cBlue)
	}
	div := divStyle.Render("│")

	var sb strings.Builder
	for i := 0; i < contentH; i++ {
		l := ""
		r := ""
		if i < len(leftLines) {
			l = leftLines[i]
		} else {
			l = line(m.paneW, "")
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		sb.WriteString(l)
		sb.WriteString(div)
		sb.WriteString(r)
		sb.WriteByte('\n')
	}
	sb.WriteString(m.statusBar())
	return sb.String()
}

// ── Left pane ─────────────────────────────────────────────────────────────────

func (m Model) leftPane(height int) []string {
	var lines []string

	// Header — dark background only under the text, not the full pane width
	lines = append(lines,
		line(m.paneW, lipgloss.NewStyle().Background(cBarBg).Foreground(cGray).Render("REQUESTS")),
	)

	// Search row
	var searchContent string
	if m.searching {
		searchContent = lipgloss.NewStyle().Foreground(cBlue).Render("/") +
			" " + m.query.Value() + "▌"
	} else {
		searchContent = lipgloss.NewStyle().Foreground(cDim).Render("/ search")
	}
	lines = append(lines, line(m.paneW, " "+searchContent))

	// Separator
	lines = append(lines, line(m.paneW,
		lipgloss.NewStyle().Foreground(cBorder).Render(strings.Repeat("─", m.paneW)),
	))

	// File list — fills the rest minus count row
	listH := height - len(lines) - 1
	if listH < 1 {
		listH = 1
	}
	lines = append(lines, m.fileListLines(listH)...)

	// Count row
	var countStr string
	if m.query.Value() != "" {
		tree := m.flatTree()
		matchFiles := 0
		for _, item := range tree {
			if item.kind == kindFile {
				matchFiles++
			}
		}
		countStr = fmt.Sprintf("%d / %d match", matchFiles, len(m.files))
	} else {
		countStr = fmt.Sprintf("%d files", len(m.files))
	}
	lines = append(lines, line(m.paneW,
		lipgloss.NewStyle().Width(m.paneW).Foreground(cDim).Render(" "+countStr),
	))

	return lines
}

func (m Model) fileListLines(height int) []string {
	items := m.flatTree()
	lines := make([]string, height)

	if len(items) == 0 {
		lines[0] = line(m.paneW,
			lipgloss.NewStyle().Foreground(cDim).Render(" No .http files found"),
		)
		for i := 1; i < height; i++ {
			lines[i] = line(m.paneW, "")
		}
		return lines
	}

	start := m.sidebarScroll
	if start > len(items) {
		start = len(items)
	}

	for i := 0; i < height; i++ {
		idx := start + i
		if idx >= len(items) {
			lines[i] = line(m.paneW, "")
			continue
		}
		item := items[idx]
		selected := idx == m.cursor
		switch item.kind {
		case kindFile:
			lines[i] = m.fileRow(item, selected)
		case kindRequest:
			lines[i] = m.reqRow(item, selected)
		}
	}
	return lines
}

func (m Model) fileRow(item treeItem, selected bool) string {
	// Display relative-to-dir for readability
	name := item.filePath
	if m.dir != "" {
		if rel, err := filepath.Rel(m.dir, item.filePath); err == nil {
			name = rel
		}
	}
	// Truncate at END (show start of string)
	maxName := m.paneW - 4
	if maxName < 1 {
		maxName = 1
	}
	if len(name) > maxName {
		name = name[:maxName-1] + "…"
	}
	expand := "▶ "
	if m.expanded[item.filePath] {
		expand = "▼ "
	}
	content := expand + name

	if selected {
		return lipgloss.NewStyle().
			Width(m.paneW).Background(cSelBg).Foreground(cBlue).Bold(true).
			Render(content)
	}
	return lipgloss.NewStyle().Width(m.paneW).Foreground(cMuted).Render(content)
}

func (m Model) reqRow(item treeItem, selected bool) string {
	// Truncate at END (show start of string)
	maxName := m.paneW - 5
	if maxName < 1 {
		maxName = 1
	}
	name := item.label
	if len(name) > maxName {
		name = name[:maxName-1] + "…"
	}
	content := "  " + name // 2-space indent for level 2

	if selected {
		return lipgloss.NewStyle().
			Width(m.paneW).Background(cSelBg).Foreground(cGreen).Bold(true).
			Render(content)
	}
	return lipgloss.NewStyle().Width(m.paneW).Foreground(cGray).Render(content)
}

// ── Right pane ────────────────────────────────────────────────────────────────

func (m Model) rightPane(w, height int) []string {
	if w < 10 {
		w = 10
	}

	// Level-1 file selected: show request list overview.
	if m.selectedFile != "" {
		return m.filePane(w, height)
	}

	var lines []string

	// ── Request section ──
	lines = append(lines, m.reqHeader(w))
	bodyH := m.reqPaneH - 1
	if bodyH < 1 {
		bodyH = 1
	}
	lines = append(lines, m.reqBody(w, bodyH)...)

	// Separator — highlight when being dragged
	sepStyle := lipgloss.NewStyle().Foreground(cBorder)
	if m.dragging == dragReqSep {
		sepStyle = lipgloss.NewStyle().Foreground(cBlue)
	}
	lines = append(lines, line(w, sepStyle.Render(strings.Repeat("─", w))))

	// ── Response section ──
	lines = append(lines, m.respHeader(w))
	respBodyH := height - len(lines)
	if respBodyH < 1 {
		respBodyH = 1
	}
	lines = append(lines, m.respBody(w, respBodyH)...)

	return lines
}

// filePane renders a request-list overview for the selected Level-1 file.
func (m Model) filePane(w, height int) []string {
	var lines []string

	// Header: "REQUESTS" (dark bg under text only) + filename badge right-aligned
	titleStyled := lipgloss.NewStyle().Background(cBarBg).Foreground(cText).Bold(true).Render(" REQUESTS")
	rel := m.selectedFile
	if m.dir != "" {
		if r, err := filepath.Rel(m.dir, m.selectedFile); err == nil {
			rel = r
		}
	}
	badge := lipgloss.NewStyle().Background(cSelBg).Foreground(cBlue).Padding(0, 1).Render(rel)
	titleW := lipgloss.Width(titleStyled)
	badgeW := lipgloss.Width(badge)
	pad := w - titleW - badgeW - 1 // 1 for trailing space
	if pad < 0 {
		pad = 0
	}
	inner := titleStyled + strings.Repeat(" ", pad) + badge + " "
	lines = append(lines, line(w, inner))

	// Separator
	lines = append(lines, line(w, lipgloss.NewStyle().Foreground(cBorder).Render(strings.Repeat("─", w))))

	// Request rows
	reqs := m.fileReqs[m.selectedFile]
	if len(reqs) == 0 {
		lines = append(lines, line(w,
			lipgloss.NewStyle().Foreground(cDim).Render("  No requests found")))
	} else {
		for i, req := range reqs {
			name := req.Name
			if name == "" {
				name = shortenURL(req.URL)
			}
			num := lipgloss.NewStyle().Foreground(cDim).Render(fmt.Sprintf("%2d", i+1))
			method := httpMethodStyle(req.Method).Render(fmt.Sprintf("%-6s", req.Method))
			nameStr := lipgloss.NewStyle().Foreground(cText).Render(name)
			row := fmt.Sprintf("  %s  %s  %s", num, method, nameStr)
			lines = append(lines, line(w, " "+row))
		}
	}

	// Fill empty lines, place hint at bottom
	for len(lines) < height-1 {
		lines = append(lines, line(w, ""))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

func (m Model) reqHeader(w int) string {
	title := lipgloss.NewStyle().Foreground(cText).Bold(true).Render("REQUEST")
	titleW := lipgloss.Width(title)

	var badge string
	var badgeW int
	if m.selected != nil {
		label := m.selected.Name
		if label == "" {
			label = m.selected.Method + " " + shortenURL(m.selected.URL)
		}
		// Truncate label so badge always fits within the header bar
		maxLabel := w - titleW - 4 // 1(leading) + title + 1(gap) + 1(pad-l) + label + 1(pad-r) + 1(trailing) – simplified
		if maxLabel < 4 {
			maxLabel = 4
		}
		if len(label) > maxLabel {
			label = label[:maxLabel-1] + "…"
		}
		badge = lipgloss.NewStyle().Background(cSelBg).Foreground(cBlue).
			Padding(0, 1).Render(label)
		badgeW = lipgloss.Width(badge)
	}

	pad := w - titleW - badgeW - 2 // 2 = leading+trailing space
	if pad < 0 {
		pad = 0
	}
	// Build exact-width inner string with explicit spaces — no Padding() wrapper
	inner := " " + title + strings.Repeat(" ", pad) + badge + " "
	return lipgloss.NewStyle().Background(cBarBg).Width(w).MaxWidth(w).Render(inner)
}

func (m Model) reqBody(w, h int) []string {
	style := lipgloss.NewStyle().Width(w-2).Padding(0, 1)

	var content string
	if m.selectedErr != nil {
		content = lipgloss.NewStyle().Foreground(cRed).
			Render("Parse error: " + m.selectedErr.Error())
	} else if m.selected == nil {
		content = lipgloss.NewStyle().Foreground(cDim).
			Render("↑↓ select a file, Enter to expand, Enter on request to run")
	} else {
		var b strings.Builder
		urlStyle := lipgloss.NewStyle().Foreground(cBlue)
		valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ce9178"))
		dimStyle := lipgloss.NewStyle().Foreground(cDim)

		// ── Variable list (shown ABOVE the request) ──────────────────────────
		// Only show variables actually used ({{name}}) in this request.
		// Declared vars show as @name=value; used-but-undeclared show dimmed.
		usedNames := extractVarNames(m.selected.URL + " " + m.selected.Body)
		for _, v := range m.selected.Headers {
			usedNames = append(usedNames, extractVarNames(v)...)
		}
		usedNames = dedupeNames(usedNames)
		if len(usedNames) > 0 {
			for _, name := range usedNames {
				val, declared := m.selected.Vars[name]
				if declared {
					varLabel := lipgloss.NewStyle().Foreground(cVar).Bold(true).Render("@" + name)
					valStr := lipgloss.NewStyle().Foreground(cGreen).Render("=" + val)
					b.WriteString(varLabel + valStr + "\n")
				} else {
					b.WriteString(dimStyle.Render("@"+name+"=") +
						lipgloss.NewStyle().Foreground(cRed).Render("(not set)") + "\n")
				}
			}
			b.WriteString(dimStyle.Render(strings.Repeat("─", 24)) + "\n")
		}

		// ── Request line ────────────────────────────────────────────────────
		fmt.Fprintf(&b, "%s %s",
			httpMethodStyle(m.selected.Method).Render(m.selected.Method),
			highlightVars(m.selected.URL, urlStyle),
		)

		// Headers
		for k, v := range m.selected.Headers {
			fmt.Fprintf(&b, "\n%s %s",
				lipgloss.NewStyle().Foreground(cGray).Render(k+":"),
				highlightVars(v, valStyle),
			)
		}

		// Body
		if m.selected.Body != "" {
			b.WriteString("\n\n")
			b.WriteString(highlightVars(m.selected.Body, valStyle))
		}
		content = b.String()
	}

	rendered := style.Render(content)
	return splitToLines(rendered, w, h, m.reqScroll)
}

func (m Model) respHeader(w int) string {
	title := lipgloss.NewStyle().Foreground(cText).Bold(true).Render("RESPONSE")
	resp := m.responses[m.selectedReqKey]
	respErr := m.errors[m.selectedReqKey]
	var right string
	switch {
	case m.inFlight[m.selectedReqKey]:
		right = lipgloss.NewStyle().Foreground(cOrange).Render("● running…")
	case respErr != nil:
		right = lipgloss.NewStyle().Foreground(cRed).Render("error")
	case resp != nil:
		right = httpStatusStyle(resp.StatusCode, resp.Status) +
			"  " + lipgloss.NewStyle().Foreground(cGray).Render(
			resp.Duration.Round(1e6).String()+" · "+byteSize(len(resp.Body)))
	}
	if m.respScroll > 0 {
		scrollNote := lipgloss.NewStyle().Foreground(cDim).Render(fmt.Sprintf(" ↑%d", m.respScroll))
		right = scrollNote + "  " + right
	}
	titleW := lipgloss.Width(title)
	rightW := lipgloss.Width(right)
	pad := w - titleW - rightW - 2
	if pad < 0 {
		pad = 0
	}
	inner := " " + title + strings.Repeat(" ", pad) + right + " "
	return lipgloss.NewStyle().Background(cBarBg).Width(w).MaxWidth(w).Render(inner)
}

func (m Model) respBody(w, h int) []string {
	style := lipgloss.NewStyle().Width(w-2).Padding(0, 1)

	resp2 := m.responses[m.selectedReqKey]
	respErr2 := m.errors[m.selectedReqKey]
	var content string
	switch {
	case m.inFlight[m.selectedReqKey]:
		content = lipgloss.NewStyle().Foreground(cOrange).Render("Sending request…")
	case respErr2 != nil:
		content = lipgloss.NewStyle().Foreground(cRed).Render("Error: " + respErr2.Error())
	case resp2 != nil:
		content = prettyJSON(resp2.Body)
		if resp2.Truncated {
			content += "\n" + lipgloss.NewStyle().Foreground(cOrange).
				Render("[Truncated >100MB — use: rest-cli run --save <file>]")
		}
	default:
		content = lipgloss.NewStyle().Foreground(cDim).Render("No response yet — press Enter to run")
	}

	rendered := style.Render(content)
	return splitToLines(rendered, w, h, m.respScroll)
}

// ── Status bar ────────────────────────────────────────────────────────────────

func (m Model) statusBar() string {
	bg := cBarBg
	bold := func(s string) string {
		return lipgloss.NewStyle().Background(bg).Foreground(cText).Bold(true).Render(s)
	}
	dim := func(s string) string {
		return lipgloss.NewStyle().Background(bg).Foreground(cGray).Render(s)
	}
	sp := func(n int) string {
		return lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", n))
	}

	keys := bold("↑↓") + dim(" nav") + sp(2) +
		bold("↵") + dim(" expand/run") + sp(2) +
		bold("/") + dim(" search") + sp(2) +
		bold("esc") + dim(" clear") + sp(2) +
		bold("e") + dim(" edit") + sp(2) +
		bold("PgUp/Dn") + dim(" scroll") + sp(2) +
		bold("[]{}") + dim(" resize") + sp(2) +
		bold("r") + dim(" refresh") + sp(2) +
		bold("q") + dim(" quit")

	var envHint string
	if len(m.env) > 0 {
		envHint = lipgloss.NewStyle().Background(bg).Foreground(cBlue).Render("env: .env.local")
	}

	keysW := lipgloss.Width(keys)
	envW := lipgloss.Width(envHint)
	pad := m.width - keysW - envW - 2
	if pad < 0 {
		pad = 0
	}

	// Padding is computed manually, so Width/MaxWidth are not needed.
	// Using them on a string with many nested styled segments causes extra height in lipgloss v1.
	return sp(1) + keys + sp(pad) + envHint + sp(1)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// extractVarNames returns {{VAR}} names found in s, in order of appearance (may contain duplicates).
func extractVarNames(s string) []string {
	var names []string
	for {
		start := strings.Index(s, "{{")
		if start == -1 {
			break
		}
		s = s[start+2:]
		end := strings.Index(s, "}}")
		if end == -1 {
			break
		}
		names = append(names, s[:end])
		s = s[end+2:]
	}
	return names
}

// dedupeNames returns names with duplicates removed, preserving first-seen order.
func dedupeNames(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := names[:0:0]
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// highlightVars renders s using base style, but highlights {{VAR}} spans in amber.
func highlightVars(s string, base lipgloss.Style) string {
	if !strings.Contains(s, "{{") {
		return base.Render(s)
	}
	var b strings.Builder
	for {
		start := strings.Index(s, "{{")
		if start == -1 {
			if s != "" {
				b.WriteString(base.Render(s))
			}
			break
		}
		if start > 0 {
			b.WriteString(base.Render(s[:start]))
		}
		s = s[start:]
		end := strings.Index(s, "}}")
		if end == -1 {
			// Unterminated {{ — treat as literal
			b.WriteString(base.Render(s))
			break
		}
		b.WriteString(lipgloss.NewStyle().Foreground(cVar).Bold(true).Render(s[:end+2]))
		s = s[end+2:]
	}
	return b.String()
}

// splitToLines splits a rendered string into exactly h lines, each w wide,
// starting at scroll offset (clamped to valid range).
func splitToLines(rendered string, w, h, scroll int) []string {
	raw := strings.Split(rendered, "\n")
	// Clamp scroll so we never go past the last line
	if maxScroll := len(raw) - h; scroll > maxScroll {
		if maxScroll > 0 {
			scroll = maxScroll
		} else {
			scroll = 0
		}
	}
	if scroll < 0 {
		scroll = 0
	}
	lines := make([]string, h)
	for i := 0; i < h; i++ {
		idx := scroll + i
		if idx < len(raw) {
			lines[i] = line(w, raw[idx])
		} else {
			lines[i] = line(w, "")
		}
	}
	return lines
}

func httpMethodStyle(method string) lipgloss.Style {
	switch method {
	case "GET":
		return lipgloss.NewStyle().Foreground(cGreen).Bold(true)
	case "POST":
		return lipgloss.NewStyle().Foreground(cOrange).Bold(true)
	case "PUT":
		return lipgloss.NewStyle().Foreground(cBlue).Bold(true)
	case "DELETE":
		return lipgloss.NewStyle().Foreground(cRed).Bold(true)
	case "PATCH":
		return lipgloss.NewStyle().Foreground(cPurple).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(cGray).Bold(true)
	}
}

func httpStatusStyle(code int, status string) string {
	var c lipgloss.Color
	switch {
	case code < 300:
		c = cGreen
	case code < 400:
		c = cOrange
	default:
		c = cRed
	}
	return lipgloss.NewStyle().Foreground(c).Bold(true).Render(status)
}

func prettyJSON(s string) string {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return s
	}
	lines := strings.Split(string(pretty), "\n")
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = colorizeJSONLine(l)
	}
	return strings.Join(out, "\n")
}

func colorizeJSONLine(l string) string {
	trimmed := strings.TrimSpace(l)
	indent := l[:len(l)-len(strings.TrimLeft(l, " "))]
	if idx := strings.Index(trimmed, `": `); idx > 0 && strings.HasPrefix(trimmed, `"`) {
		key := trimmed[:idx+1]
		rest := strings.TrimSpace(trimmed[idx+2:])
		return indent +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9cdcfe")).Render(key) +
			": " + colorizeJSONValue(rest)
	}
	return indent + colorizeJSONValue(trimmed)
}

func colorizeJSONValue(s string) string {
	trail := ""
	val := s
	if strings.HasSuffix(val, ",") {
		trail = ","
		val = val[:len(val)-1]
	}
	var out string
	switch {
	case strings.HasPrefix(val, `"`):
		out = lipgloss.NewStyle().Foreground(lipgloss.Color("#ce9178")).Render(val)
	case val == "true" || val == "false":
		out = lipgloss.NewStyle().Foreground(lipgloss.Color("#569cd6")).Render(val)
	case val == "null":
		out = lipgloss.NewStyle().Foreground(cGray).Render(val)
	case len(val) > 0 && (val[0] >= '0' && val[0] <= '9' || val[0] == '-'):
		out = lipgloss.NewStyle().Foreground(lipgloss.Color("#b5cea8")).Render(val)
	default:
		out = val
	}
	return out + trail
}

func shortenURL(u string) string {
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	if len(u) > 34 {
		return u[:31] + "..."
	}
	return u
}

func byteSize(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%db", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fkb", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fmb", float64(n)/1024/1024)
	}
}
