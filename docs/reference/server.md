# Server Security & Configuration Guide

This guide provides server-side best practices and configuration details for authentication and API security.

## Transport Security (Production)
- Terminate TLS and expose the API only over HTTPS in production.
- Maintain valid certificates and prefer strong TLS versions/ciphers.
- Avoid enabling insecure HTTP endpoints; if you must for health checks, keep them internal.

## JWT Configuration
- Secrets:
  - Set `TRAZR_JWT_SECRET` (or use `TRAZR_JWT_SECRETS` with `TRAZR_JWT_KID` for key rotation).
  - Do not commit secrets to source control; use a secret manager.
- Claims:
  - Set `TRAZR_JWT_ISSUER` and `TRAZR_JWT_AUDIENCE` and keep them stable across deployments.
- Lifetime:
  - Configure access token TTL via `TRAZR_JWT_TTL` (Go duration e.g., `60m`, `24h`). Default is `24h`.
  - Prefer shorter TTLs in production (e.g., 15–60 minutes) and rely on renewal.
  - Implement renewal endpoint `/api/v1/auth/renew` and ensure appropriate rate limiting.

## Key Rotation
- Use `TRAZR_JWT_SECRETS` (comma-separated `kid=secret`) and set `TRAZR_JWT_KID` for the active signing key.
- During rotation, keep the old key in `TRAZR_JWT_SECRETS` so existing tokens validate until they expire.
- After the overlap window, remove the old key.

## Token Revocation (Denylist)
- Token denylist is persisted in DB (`trazr_auth_token_blacklist`).
- On logout or forced revocation, store the `jti` with `expires_at` to invalidate the token until it naturally expires.
- Periodically run cleanup to remove expired entries (`CleanupExpiredBlacklistedTokens`).

## Login Security
- Lockout / throttling: enable and tune login attempt tracking to prevent brute-force attacks.
- Use strong password hashing (bcrypt, argon2). Ensure cost settings are appropriate for your environment.
- Audit events: record login success/failure, lockouts.

## CORS and Headers
- Restrict `Access-Control-Allow-Origin` to trusted domains in production.
- Only allow necessary methods and headers.
- Disable caching for sensitive endpoints when appropriate.

## Bootstrap User (Development Only)
- The bootstrap flow is for local/dev environments.
- Env flags:
  - `TRAZR_BOOTSTRAP_USER=true`
  - `TRAZR_BOOTSTRAP_USER_EMAIL`, `TRAZR_BOOTSTRAP_USER_PASSWORD`
  - `TRAZR_BOOTSTRAP_USER_ROLE` (optional)
- Implementation:
  - `trazrextension/extension.go` invokes `pkg/auth.BootstrapUser(...)` after DB init.
  - `pkg/auth/bootstrap.go` ensures tables exist and creates the user if missing (idempotent).
- Do not enable in production.

## Observability & Logging
- Do not log credentials or tokens. Scrub `Authorization` headers and sensitive fields.
- Emit structured logs with minimal necessary context.
- Collect metrics on auth endpoints (latency, error rates, lockouts, renewals).

## Configuration Checklist
- [ ] HTTPS/TLS enabled at the edge
- [ ] `TRAZR_JWT_SECRET` or `TRAZR_JWT_SECRETS` + `TRAZR_JWT_KID` set
- [ ] `TRAZR_JWT_ISSUER` and `TRAZR_JWT_AUDIENCE` set
- [ ] `TRAZR_JWT_TTL` set appropriately for environment (e.g., `60m` in prod)
- [ ] Login rate limiting/lockout enabled and tuned
- [ ] Denylist table and cleanup job configured
- [ ] CORS restricted to trusted origins
- [ ] Bootstrap user disabled in production

## References
- OWASP JSON Web Token Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/JSON_Web_Token_for_Java_Cheat_Sheet.html
- OWASP REST Security Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html
- OWASP WSTG - Testing JSON Web Tokens: https://owasp.org/www-project-web-security-testing-guide/latest/4-Web_Application_Security_Testing/06-Session_Management_Testing/10-Testing_JSON_Web_Tokens

---

## Production Hardening Improvements

- __TLS & Edge__
  - Enforce HTTPS everywhere; enable HSTS at the edge/CDN.
  - Place a WAF/API gateway in front (rate limit per IP/account, bot protection).

- __JWT TTL & Config__
  - Shorten access token TTL (15–60m) via config; renew using `/api/v1/auth/renew`.
  - Set and enforce `TRAZR_JWT_ISSUER` and `TRAZR_JWT_AUDIENCE`.
  - Implement key rotation runbook using `TRAZR_JWT_SECRETS` + `TRAZR_JWT_KID` with overlap.

- __Login Protections__
  - Enable login throttling/lockout; add per-IP and per-account counters.
  - Use strong password hashing params (bcrypt/argon2) and rotate costs over time.
  - Optionally enforce MFA for admin roles.

- __CORS & Headers__
  - Restrict CORS origins to an allowlist in prod; block `*`.
  - Add security headers where applicable (X-Content-Type-Options, Frame-Ancestors, CSP for any web UI).

- __Secrets & Config Management__
  - Source secrets from a secret manager (not env files in prod images).
  - Rotate secrets regularly; avoid logging config/env dumps in production.

- __Token Revocation & Cleanup__
  - Ensure periodic job runs `CleanupExpiredBlacklistedTokens`.
  - Provide an admin endpoint/runbook to revoke tokens by `jti` on incident.

- __Observability__
  - Emit metrics on auth endpoints (QPS, latencies, error rates, lockouts, renewals, 401/403 counts).
  - Add alerts for spikes in failures or lockouts.

- __Operational Hygiene__
  - Back up the auth database and test restore; secure backups.
  - Minimize admin surface; require `admin` role for user listing/deletion; log all admin actions.
  - Keep dependencies patched; run regular SCA and vulnerability scans.
