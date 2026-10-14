# CLI Authentication Integration Guide

This guide explains how to update the CLI to obtain a JWT dynamically by logging in, instead of using a hard-coded token.

The server exposes HTTP endpoints under the extension at `http://localhost:18888` by default (see `trazrextension/extension.go`).

## Summary
- POST `/api/v1/auth/login` with email and password to get a JWT.
- Send `Authorization: Bearer <token>` with subsequent requests.
- Optionally use `/api/v1/auth/renew` to refresh a token.
- For local dev, you can bootstrap a demo user via env vars.

## Local Development Bootstrap (optional)
For local development, the server can auto-create a demo user on startup. This is controlled via environment variables and is idempotent.

Set these in your run script (e.g., `script/macos-run.sh`):

```bash
export TRAZR_JWT_SECRET="development-only-secret-change-me"
export TRAZR_BOOTSTRAP_USER=true
export TRAZR_BOOTSTRAP_USER_EMAIL="demo@trazr.local"
export TRAZR_BOOTSTRAP_USER_PASSWORD="demo123"
export TRAZR_BOOTSTRAP_USER_ROLE=admin
```

Then start:

```bash
./trazr-otel/trazr-otel --config config.yaml
```

The CLI can then log in with the bootstrapped user without manual registration.

## Endpoints
- POST `/api/v1/auth/login`
  - Request JSON: `{ "email": "...", "password": "..." }`
  - Response JSON: `{ "token": "<jwt>", "roles": ["..."] , "user": { ... } }`
- GET `/api/v1/auth/whoami` (requires Authorization header) — verify token.
- POST `/api/v1/auth/renew` (requires Authorization header) — get a fresh token.

## Minimal Flow (Shell)
```bash
API_BASE=${API_BASE:-http://localhost:18888}
EMAIL=${TRAZR_CLI_EMAIL:-demo@trazr.local}
PASSWORD=${TRAZR_CLI_PASSWORD:-demo123}

# 1) Login to obtain a token
TOKEN=$(curl -sS -X POST "$API_BASE/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}" \
  | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')

if [ -z "$TOKEN" ]; then
  echo "Failed to obtain token" >&2
  exit 1
fi

# 2) Use token in subsequent requests
curl -sS "$API_BASE/api/v1/auth/whoami" -H "Authorization: Bearer $TOKEN"
```

## Minimal Flow (Go)
```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
)

type loginResp struct {
    Token string `json:"token"`
}

func main() {
    api := getenv("API_BASE", "http://localhost:18888")
    email := getenv("TRAZR_CLI_EMAIL", "demo@trazr.local")
    password := getenv("TRAZR_CLI_PASSWORD", "demo123")

    tok, err := login(api, email, password)
    if err != nil { panic(err) }

    req, _ := http.NewRequest(http.MethodGet, api+"/api/v1/auth/whoami", nil)
    req.Header.Set("Authorization", "Bearer "+tok)
    resp, err := http.DefaultClient.Do(req)
    if err != nil { panic(err) }
    defer resp.Body.Close()
    b, _ := io.ReadAll(resp.Body)
    fmt.Println(string(b))
}

func login(api, email, password string) (string, error) {
    body, _ := json.Marshal(map[string]string{"email": email, "password": password})
    resp, err := http.Post(api+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
    if err != nil { return "", err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("login failed: %s", resp.Status)
    }
    var lr loginResp
    if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil { return "", err }
    if lr.Token == "" { return "", fmt.Errorf("empty token in response") }
    return lr.Token, nil
}

func getenv(k, def string) string {
    if v := os.Getenv(k); v != "" { return v }
    return def
}
```

## Token Renewal (optional)
If your CLI runs long-lived operations, renew the token before it expires:

```bash
NEW_TOKEN=$(curl -sS -X POST "$API_BASE/api/v1/auth/renew" -H "Authorization: Bearer $TOKEN" | sed -n 's/.*"token":"\([^"\]*\)".*/\1/p')
if [ -n "$NEW_TOKEN" ]; then TOKEN="$NEW_TOKEN"; fi
```

## Reconnects and Token Lifecycle
- Reuse the existing token after server restarts as long as it is still valid and the server JWT secret(s) did not change.
- Track token expiry (`exp`) and renew proactively.
- On any 401 Unauthorized:
  - First try `POST /api/v1/auth/renew` with the current token.
  - If renew fails (still 401/403), perform a fresh login.

Recommended strategy:
- Treat the token as expiring slightly earlier (e.g., 60s safety margin) to account for clock skew.
- Proactively renew when remaining lifetime < 5 minutes (or at ~85% of lifetime).
- Add jitter (±30s) to avoid synchronized renewals across multiple clients.

Minimal pseudocode:
```
if token.empty() || time.Now() > exp-5m {
    token = login(email, password)
    exp = parseExp(token)
}

doRequest() with Authorization: Bearer token
if response.status == 401 {
    newToken = renew(token)
    if newToken.ok {
        token = newToken
        exp = parseExp(token)
        retry request once
    } else {
        token = login(email, password)
        exp = parseExp(token)
        retry request once
    }
}
```

Parsing `exp` from JWT (Go, without verification):
```go
func parseExpFromJWT(tok string) (time.Time, error) {
    parts := strings.Split(tok, ".")
    if len(parts) < 2 { return time.Time{}, fmt.Errorf("bad token") }
    payload, err := base64.RawURLEncoding.DecodeString(parts[1])
    if err != nil { return time.Time{}, err }
    var raw struct{ Exp int64 `json:"exp"` }
    if err := json.Unmarshal(payload, &raw); err != nil { return time.Time{}, err }
    return time.Unix(raw.Exp, 0), nil
}
```

## Error Handling Notes
- 401 Unauthorized: token missing/expired/invalid.
- 403 Forbidden: user lacks required role/permissions for a given admin endpoint.
- Network/connection errors: verify the extension is listening at `:18888`.

## Client Configuration via Environment
Recommended CLI envs (client-side only):
- `API_BASE` — default `http://localhost:18888` (use HTTPS in production)
- `TRAZR_CLI_EMAIL` — CLI login email (for local dev)
- `TRAZR_CLI_PASSWORD` — CLI login password (for local dev)

## Client Transport Security
- Use HTTPS for `API_BASE` in production. The `http://localhost:18888` examples are for local development only.
- Do not disable TLS verification in clients.

## Client Token Storage
- Prefer in-memory storage. If caching to disk, restrict permissions (e.g., 0600) and avoid shared locations.
- Never log tokens or Authorization headers; redact in any debug output.
- Clear cached tokens on logout/exit or after prolonged inactivity.

## Migration from Hard-Coded Token (Client)
1. Remove any hard-coded token usage from the CLI.
2. Implement login at startup against `/api/v1/auth/login`.
3. Store the token in memory and attach `Authorization: Bearer <token>` to all requests.
4. Add proactive renewal (see Reconnects and Token Lifecycle) and retry-once logic.
5. Provide env overrides (`API_BASE`, `TRAZR_CLI_EMAIL`, `TRAZR_CLI_PASSWORD`) for local dev.
