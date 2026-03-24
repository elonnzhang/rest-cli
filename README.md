# rest-cli

A terminal HTTP client for `.http` files — the ones you already have in your project.

Every other terminal HTTP client (Posting, Slumber, Hurl, Bruno) makes you migrate to a new format. rest-cli runs the `.http` files from VS Code REST Client and JetBrains without modification.

<!-- demo GIF goes here -->

## Install

```bash
go install github.com/elonnzhang/rest-cli@latest
```

Or download a pre-built binary from [Releases](https://github.com/elonnzhang/rest-cli/releases).

## Usage

```bash
# Launch the TUI — fuzzy-browse all .http files in the current directory
rest-cli

# Run a specific file from the CLI (CI/scripting friendly)
rest-cli run foo.http

# Run a named request
rest-cli run -n "Login" auth.http

# Use an extra env file
rest-cli run --env .env.prod auth.http

# Search a different directory
rest-cli --dir ./api
```

## .http file format

rest-cli supports the [VS Code REST Client](https://marketplace.visualstudio.com/items?itemName=humao.rest-client) format:

```http
### Login
POST https://api.example.com/auth/login
Content-Type: application/json
Authorization: Bearer {{API_TOKEN}}

{"email": "user@example.com", "password": "secret"}

###

### Get user
GET https://api.example.com/users/me
Authorization: Bearer {{API_TOKEN}}
```

Multiple requests per file are separated by `###`. Variables use `{{VAR}}` syntax and are loaded from `.env` and `.env.local` files at execution time.

## Environment variables

rest-cli loads env files in this order (later values override earlier ones):

1. `.env` in the current directory
2. `.env.local` in the current directory
3. `--env <file>` flag (highest priority)

```bash
# .env
API_URL=https://api.example.com
API_TOKEN=dev-token
```

## TUI keybindings

| Key | Action |
|-----|--------|
| `↑↓` | Navigate file list |
| `Enter` | Run selected request |
| `/` | Fuzzy search |
| `Esc` | Clear search |
| `r` | Refresh file list |
| `e` | Open file in `$EDITOR` |
| `h` | Toggle history |
| `Ctrl+C` | Cancel in-flight request |
| `q` | Quit |

## CLI exit codes

| Code | Meaning |
|------|---------|
| `0` | Response received (any HTTP status) |
| `1` | Fatal error (file not found, parse error, network error, timeout) |

HTTP 4xx/5xx responses exit `0` — check the status in the output if needed.

## Build from source

```bash
git clone https://github.com/elonnzhang/rest-cli
cd rest-cli
go build -o rest-cli ./cmd/...
```

## License

MIT
