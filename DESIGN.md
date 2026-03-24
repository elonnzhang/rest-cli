# TUI Design — rest-cli

This document is the source of truth for all TUI layout and styling decisions.
All changes to `internal/tui/view.go` and `internal/tui/model.go` MUST conform to this spec.

---

## Layout

```
┌──── LEFT PANE (30 cols) ────┬──── RIGHT PANE (remaining cols) ───────────────┐
│ REQUESTS                    │ REQUEST                      [Get my IP]        │
│ / search                    ├────────────────────────────────────────────────  │
│ ─────────────────────────── │ GET  https://httpbin.org/ip                     │
│▼ demo.http          ← L1    │ Accept: application/json                        │
│  Get my IP          ← L2    │                                                 │
│  Get headers        ← L2    │                                                 │
│  Post JSON          ← L2    │                                                 │
│▶ get.http           ← L1    │                                                 │
│▶ post.http          ← L1    ├────────────────────────────────────────────────  │
│                             │ RESPONSE                   200 OK  142ms · 42b  │
│                             │                                                  │
│                             │ {                                                │
│                             │   "origin": "1.2.3.4"                           │
│ 3 files                     │ }                                                │
├─────────────────────────────┴────────────────────────────────────────────────  │
│ ↑↓ nav  ↵ expand/run  / search  esc clear  r refresh  e edit  q quit         │
└──────────────────────────────────────────────────────────────────────────────  ┘
```

---

## Two-Level Tree (Sidebar)

The sidebar shows a two-level tree:

| Level | Item type | Indicator | Enter action |
|-------|-----------|-----------|-------------|
| 1 | `.http` file | `▶` (collapsed) / `▼` (expanded) | Toggle expand/collapse |
| 2 | Request within file | `  ` (2-space indent) | Execute the request |

### Tree rules
- All files start **collapsed** — Level 1 only
- `Enter` on a **Level 1** file → expand to show its requests; `Enter` again → collapse
- `Enter` on a **Level 2** request → send the HTTP request
- Requests are parsed lazily — only when a file is first expanded (or navigated to)
- Parsed requests are cached in `m.fileReqs[path]`; cleared on `r` (refresh) or `e` (edit)

### Display names
- File rows: path relative to `m.dir` (e.g., `demo.http` not `/full/abs/path.http`)
- Request rows: `req.Name` if set; otherwise `METHOD shortenURL(URL)`
- Names longer than `leftW-4` are truncated with `…` prefix

---

## Dimensions

| Constant | Value | Description |
|----------|-------|-------------|
| `leftW`  | 30    | Sidebar width in terminal columns (fixed) |
| `reqH`   | 9     | Lines for the request pane (1 header + 8 body lines) |
| `rightW` | `width - leftW - 1` | Right pane width (dynamic, minus divider) |
| `contentH` | `height - 1` | All rows minus status bar |
| `sidebarListH` | `height - 5` | Visible rows for tree items |

### sidebarListH formula
`contentH(height-1)` minus `header(1)` + `search(1)` + `sep(1)` + `count(1)` = `height - 5`

---

## Component Map

```
View()
├── leftPane(contentH) → []string
│   ├── header row        "REQUESTS" label on cBarBg
│   ├── search row        "/ search" dim, or "/ <query>▌" when active
│   ├── separator         ─────────────────────────────── (cBorder)
│   ├── fileListLines()   two-level tree, scrollable, fills available height
│   │   ├── fileRow()     Level 1: "▶ filename" or "▼ filename"
│   │   └── reqRow()      Level 2: "  request name" (2-space indent)
│   └── count row         "3 files" or "2 / 5 match" (cDim)
│
├── rightPane(rightW, contentH) → []string
│   ├── reqHeader(w)      "REQUEST" bold + [name badge] right-aligned
│   ├── reqBody(w, 8)     method + URL + headers + body (8 lines fixed)
│   ├── separator         ─────────────────────────────── (cBorder)
│   ├── respHeader(w)     "RESPONSE" bold + status + timing right-aligned
│   └── respBody(w, H)    JSON / error / placeholder (fills remaining height)
│
└── statusBar()           single line, full terminal width
    ↑↓ nav  ↵ expand/run  / search  esc clear  r refresh  e edit  q quit
```

---

## Keybindings

| Key | Context | Action |
|-----|---------|--------|
| `↑` / `↓` | Normal | Move cursor up/down in tree |
| `Enter` | Level 1 file selected | Toggle expand/collapse |
| `Enter` | Level 2 request selected | Execute the HTTP request |
| `Enter` | Search active | Execute top matching request |
| `/` | Normal | Activate fuzzy search |
| `Esc` | Search active | Clear search, return to normal |
| `r` | Normal | Refresh file list (clears tree + cache) |
| `e` | Normal | Open selected `.http` file in `$EDITOR` |
| `q` | Normal | Quit |
| `Ctrl+C` | In-flight | Cancel in-flight request |
| `Ctrl+C` | Idle | Quit |

---

## Mouse Support

Enabled via `tea.WithMouseCellMotion()` in `cmd/main.go`.

| Action | Where | Result |
|--------|-------|--------|
| Left click (first) | Sidebar item | Move cursor to that item, load preview |
| Left click (second, same item) | File row | Toggle expand/collapse |
| Left click (second, same item) | Request row | Execute the request |
| Scroll wheel up | Sidebar | Scroll sidebar up |
| Scroll wheel down | Sidebar | Scroll sidebar down |

**Mouse coordinate mapping:**
- Sidebar: columns `0..leftW-1`
- Tree items start at row `3` (rows 0/1/2 = header/search/separator)
- `itemIdx = mouseY - 3 + m.sidebarScroll`

---

## Color System

| Token    | Hex       | Used for |
|----------|-----------|---------|
| `cBlue`  | `#4a9eff` | Selected file row, URLs, search `/`, PUT method |
| `cGreen` | `#4caf50` | GET method, 2xx status, selected request row |
| `cOrange`| `#ff9800` | POST method, 3xx status, in-flight `● running…` |
| `cRed`   | `#f44336` | DELETE method, 4xx/5xx status, errors |
| `cPurple`| `#9c27b0` | PATCH method |
| `cGray`  | `#888888` | Header keys, unselected request rows |
| `cDim`   | `#555555` | Placeholder text, `─` separator lines |
| `cText`  | `#d0d0d0` | Primary text: REQUEST / RESPONSE labels |
| `cMuted` | `#999999` | Unselected file rows |
| `cSelBg` | `#1d3557` | Selected row background, request name badge |
| `cBarBg` | `#1a1a1a` | Section header bars, status bar background |
| `cBorder`| `#444444` | `│` divider, `─` separators |

---

## Typography (terminal)

Hierarchy via:
- **Bold** — section labels, HTTP methods, keybinding keys, selected row text
- **Dim / muted color** — secondary text, placeholders, unselected items
- **Color** — method color-coding, status codes, JSON syntax highlighting
- **Background** — selected row (`cSelBg`), header bars (`cBarBg`)

---

## Three Rules (never break these)

### Rule 1 — Every line must be exactly `w` columns wide

```go
func line(w int, s string) string {
    return lipgloss.NewStyle().Width(w).MaxWidth(w).Render(s)
}
```

### Rule 2 — Panes are `[]string` slices, zipped manually

```go
for i := 0; i < contentH; i++ {
    sb.WriteString(leftLines[i])  // exactly leftW cols
    sb.WriteString(div)           // "│" — 1 col
    sb.WriteString(rightLines[i]) // exactly rightW cols
    sb.WriteByte('\n')
}
```

**Never use:** `lipgloss.JoinHorizontal()`, `lipgloss.Border()` on sidebar elements.

### Rule 3 — Status bar is inline text, no box styling

```go
keys := bold("↑↓") + dim(" nav") + "  " + bold("↵") + dim(" expand/run") + ...
```

**Never use `RoundedBorder()` on key badges** — makes them 3 lines tall.

---

## JSON Syntax Highlighting

| Token | Color |
|-------|-------|
| Object keys | `#9cdcfe` (light blue) |
| String values | `#ce9178` (orange-brown) |
| Boolean values | `#569cd6` (blue) |
| Null | `cGray` |
| Numbers | `#b5cea8` (light green) |

---

## States

### Sidebar — Level 1 (file row)
| State    | Style |
|----------|-------|
| Normal, collapsed | `cMuted`, `▶` prefix |
| Normal, expanded  | `cMuted`, `▼` prefix |
| Selected, collapsed | `cBlue` bold on `cSelBg`, `▶` prefix |
| Selected, expanded  | `cBlue` bold on `cSelBg`, `▼` prefix |

### Sidebar — Level 2 (request row)
| State    | Style |
|----------|-------|
| Normal   | `cGray`, 2-space indent |
| Selected | `cGreen` bold on `cSelBg`, 2-space indent |

### Response header
| State      | Right-side indicator |
|------------|---------------------|
| Idle       | (empty) |
| In-flight  | `● running…` in `cOrange` |
| Error      | `error` in `cRed` |
| 2xx        | status in `cGreen` bold + timing in `cGray` |
| 3xx        | status in `cOrange` bold + timing |
| 4xx/5xx    | status in `cRed` bold + timing |

---

## What Was Tried and Rejected

| Approach | Why it failed |
|----------|--------------|
| `lipgloss.JoinHorizontal` on styled panes | Collapsed or misaligned columns |
| `BorderRight(true)` on each sidebar element | Inconsistent column widths |
| `RoundedBorder()` on status bar key badges | Each badge becomes 3 lines tall |

**Current approach (v3 + tree) is the correct one.**
