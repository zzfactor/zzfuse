# Authentication and Authorization – Go Best Practices for Trazr

This document captures battle-tested, idiomatic Go guidance for implementing and operating authentication and authorization in this repo. It is tailored to `pkg/auth` and based on authoritative sources.

## Principles

- Prefer simplicity, clarity, and security-by-default
- Separate concerns: handlers → services → repositories → crypto/utilities
- Make behavior observable and testable; avoid global state; inject dependencies
- Defend against abuse: rate limits, lockout, CSRF, replay, sidejacking

## Architecture and Boundaries

- Define small interfaces per responsibility:
  - `UserRepository`, `IdentityRepository`, `RoleRepository`, `TokenDenylist`, `AuditLogger`, `EmailSender`, `Clock`, `CryptoRng`
- Implement `AuthService` that composes these interfaces; keep HTTP handlers thin
- Centralize config via constructor parameters; no package-level mutable state

## Identity and Accounts

- Normalize emails: trim, lower-case, Unicode NFC; validate length and pattern
- Store PII with length constraints; use `TEXT CHECK (length(col) <= N)` where appropriate
- Add `email_verified BOOLEAN NOT NULL DEFAULT FALSE`

## Passwords

- Hash with Argon2id (preferred) or bcrypt with elevated cost (≥ 12)
- Add server-side pepper (KMS/secret manager); rotate peps with dual validation window
- Enforce password policy (length, complexity, breach check optional)
- Rehash-on-login when parameters increase (argon2id ops/mem/time or bcrypt cost)

## Sessions and JWT

- Issue short-lived Access Tokens (5–15m) and longer Refresh Tokens (hours–days) when needed
- Claims to include: `iss`, `sub` (user id string), `aud`, `iat`, `nbf`, `exp`, `jti`; optional `roles`, minimal PII
- Validation:
  - Require expected algorithm; reject `none`
  - Validate time-based claims with small leeway (e.g., 5s)
  - Check audience/issuer
  - Check denylist for `jti` on every request
- Key Management:
  - Prefer asymmetric keys (RS256/ES256) for token verification by multiple services; use `kid` header
  - Maintain key set with rotation; publish JWKS endpoint when verifying remotely (can use `micahparks/keyfunc` for JWKs fetching)
  - For HS256, rotate secrets and keep previous secrets available until expiry
- Sidejacking mitigation (optional):
  - Bind token to a server-set cookie fingerprint claim (hashed) and verify it on each request (OWASP guidance)

References:
- golang-jwt/jwt v5 migration and validator usage; require alg and use Validator with leeway
  - [golang-jwt/jwt v5](https://github.com/golang-jwt/jwt) (see migration and validator snippets)
- [OWASP JWT Cheat Sheet](https://github.com/owasp/cheatsheetseries)

## Token Denylist (Revocation)

- Store `jti` + `expires_at` only; never store whole token
- Use `TIMESTAMPTZ`; unique index on `jti`; background cleanup of expired rows
- Optionally cache `jti` in Redis for fast checks; TTL = time to `exp`

## OAuth2 / OpenID Connect (GitHub, Google)

- Use `golang.org/x/oauth2` and `coreos/go-oidc` for Google ID Token validation
- Always implement and verify `state` (cryptographically random; store in secure, HttpOnly, SameSite cookie)
- Prefer PKCE for public clients (SPAs/mobile)
- Account linking on first login: if provider email is verified and matches an existing user, link identity instead of creating duplicates (transactional)
- Store provider subject as `provider_user_id`; unique `(provider, provider_user_id)`
- Do not create placeholder passwords for OAuth-only accounts; allow `password_hash NULL` with a separate `auth_method`

References:
- [golang.org/x/oauth2](https://pkg.go.dev/golang.org/x/oauth2)
- [coreos/go-oidc](https://github.com/coreos/go-oidc)
- OWASP Authentication Cheat Sheet

## MFA (TOTP)

- Store MFA secrets encrypted-at-rest (envelope encryption / KMS)
- Validate TOTP with small skew; rate-limit verification attempts
- Provide recovery (scratch) codes hashed and one-time-use

## Lockout and Rate Limiting

- Lockout: windowed failed-attempt counter (e.g., 5/15m) and backoff
- Avoid user enumeration: same generic error and consistent timing
- Track by user and optionally IP; clear counters on success
- Apply API-level rate limits (IP/user/action) with distributed store

References:
- OWASP Authentication Cheat Sheet (timing/user enumeration examples)

## Email Verification and Password Reset

- Token generation: 32+ bytes from CSPRNG; store SHA-256 of token, not plaintext
- Enforce expiry (e.g., 15–60m) and single-use; cleanup job for expired/used
- Email verification gates sensitive operations (or login) until verified

## RBAC / Authorization

- Keep roles minimal; consider permissions later if necessary
- Provide helpers/middleware to extract claims and check roles
- For long sessions, prefer checking DB for admin actions even if roles are in token

## Cookies and CSRF (for browser flows)

- Prefer HttpOnly, Secure, SameSite=Strict/Lax for cookies
- If using cookies for refresh/session, implement CSRF protection (double-submit or SameSite + custom header)
- Avoid `localStorage` for long-lived tokens; keep access tokens in memory

## Database and Schema Recommendations

- Use `TIMESTAMPTZ` for all timestamps; default `NOW()`
- Add `CHECK` constraints and proper indexes; enforce uniqueness
- `GENERATED BY DEFAULT AS IDENTITY` over `SERIAL` for new tables

## Logging and Observability

- Structured logs (JSON). Log auth events at appropriate levels (avoid sensitive data)
- Add OTel spans for login, token issue/validate, OAuth callbacks, DB calls; annotate user id (not email) and outcomes

## Testing

- Table-driven unit tests; fuzz inputs (tokens, emails)
- Integration tests for OAuth flows with stub providers; TOTP verification; lockout edge cases
- Verify first-user admin path is race-free

## Configuration

- Load secrets from env/secret manager; never hardcode
- Centralize TTLs, bcrypt/argon params, lockout thresholds, allowed audiences/issuers
- Support key rotation via config (active KID + previous KIDs)

---

## Application to Current Codebase (Actionable Plan)

- JWT
  - Add `iss`, `aud`, `nbf`, `jti` to claims; use `jwt.NewValidator` with leeway; enforce alg method
  - Load keys from config; support rotation and `kid`; prefer RS256/ES256
- Denylist
  - Standardize schema to `jti` + `expires_at`; change code to store/check `jti`
- Login Flow
  - Integrate `IsAccountLocked` check; record attempts and clear on success
  - Enforce MFA step when enabled
  - Add audit events for success/failure
- OAuth
  - Implement `state`; add PKCE where relevant; link existing users by verified email
  - Wrap user-create + identity-link in a transaction
  - Remove placeholder passwords for OAuth users
- Passwords
  - Switch to argon2id or increase bcrypt cost; add rehash-on-login
  - Add policy validation and email normalization
- Tables
  - Fix column mismatches (`attempted_at` vs `timestamp`; audit columns), use `TIMESTAMPTZ`
- Rate Limit
  - Correct key derivation; move to Redis-backed limiter for multi-instance
- Secrets
  - Remove hardcoded `jwtSecret`; inject via config; rotate with `kid`
- Emails
  - Store hashed reset/verify tokens; remove debug prints; add cleanup job
- MFA
  - Encrypt secrets at rest; add scratch codes

---

## References

- golang-jwt/jwt v5 (validation, parser, keyfunc, validator): [jwt docs](https://github.com/golang-jwt/jwt)
- Go OAuth2: [pkg.go.dev golang.org/x/oauth2](https://pkg.go.dev/golang.org/x/oauth2)
- Go OIDC: [coreos/go-oidc](https://github.com/coreos/go-oidc)
- OWASP Cheat Sheet Series: [Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html), [JWT](https://cheatsheetseries.owasp.org/cheatsheets/JSON_Web_Token_Cheat_Sheet_for_Java.html), [Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html), [Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html) 