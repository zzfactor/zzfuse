package client

import (
    "context"
    "encoding/base64"
    "encoding/json"
    "errors"
    "net/http"
    "strings"
    "sync"
    "time"
)

// TokenProvider exchanges API keys for short-lived JWTs and caches them until near expiry.
// It is safe for concurrent use.
type TokenProvider struct {
    baseURL     string
    httpClient  *http.Client
    refreshSkew time.Duration

    mu    sync.RWMutex
    cache map[string]cachedToken
}

type cachedToken struct {
    token  string
    expiry time.Time
}

// New creates a TokenProvider. If httpClient is nil, a default client with 15s timeout is used.
func New(baseURL string, httpClient *http.Client, refreshSkew time.Duration) *TokenProvider {
    if httpClient == nil {
        httpClient = &http.Client{Timeout: 15 * time.Second}
    }
    return &TokenProvider{
        baseURL:     strings.TrimRight(baseURL, "/"),
        httpClient:  httpClient,
        refreshSkew: refreshSkew,
        cache:       make(map[string]cachedToken),
    }
}

// GetToken returns a JWT for the provided apiKey, refreshing it if missing or near expiry.
func (p *TokenProvider) GetToken(ctx context.Context, apiKey string) (string, error) {
    if apiKey == "" {
        return "", errors.New("api key is required")
    }
    // Fast path: read from cache
    p.mu.RLock()
    ct, ok := p.cache[apiKey]
    p.mu.RUnlock()
    if ok {
        if ct.expiry.IsZero() || time.Until(ct.expiry) > p.refreshSkew {
            return ct.token, nil
        }
    }
    // Refresh via exchange endpoint
    tok, exp, err := p.exchange(ctx, apiKey)
    if err != nil {
        return "", err
    }
    p.mu.Lock()
    p.cache[apiKey] = cachedToken{token: tok, expiry: exp}
    p.mu.Unlock()
    return tok, nil
}

func (p *TokenProvider) exchange(ctx context.Context, apiKey string) (string, time.Time, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/v1/auth/apikeys/exchange", http.NoBody)
    if err != nil {
        return "", time.Time{}, err
    }
    req.Header.Set("X-API-Key", apiKey)

    resp, err := p.httpClient.Do(req)
    if err != nil {
        return "", time.Time{}, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return "", time.Time{}, errors.New("apikey exchange failed: status " + resp.Status)
    }
    var out struct{
        Token string `json:"token"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return "", time.Time{}, err
    }
    exp := parseExp(out.Token)
    return out.Token, exp, nil
}

// parseExp extracts the exp claim from a JWT without verifying signature.
func parseExp(jwt string) time.Time {
    parts := strings.Split(jwt, ".")
    if len(parts) < 2 { return time.Time{} }
    payload := parts[1]
    // base64url raw
    b, err := base64.RawURLEncoding.DecodeString(payload)
    if err != nil { return time.Time{} }
    var claims struct{ Exp int64 `json:"exp"` }
    if err := json.Unmarshal(b, &claims); err != nil { return time.Time{} }
    if claims.Exp == 0 { return time.Time{} }
    return time.Unix(claims.Exp, 0)
}
