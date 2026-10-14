# Auth Package

This package provides JWTs, password hashing, roles, reset tokens, SMTP delivery, OAuth helpers, and storage-backed authentication operations.

## Configuration (Environment Variables)

The auth package is driven by environment variables. The following lists each variable, whether it is required, and what it affects.

### JWT (stateless sessions)
- TRAZR_JWT_SECRET (required)
  - HS256 signing/validation secret. The process will panic if not set.
  - Keep this secret long and random; rotate periodically.
- TRAZR_JWT_SECRETS (optional)
  - Rotation map: comma-separated `kid=secret` entries (e.g., `v1=base64secret1,v2=base64secret2`).
  - Used for verification (by kid) and for signing when TRAZR_JWT_KID is set.
- TRAZR_JWT_KID (optional but recommended when rotating)
  - Selects which kid from TRAZR_JWT_SECRETS to sign new tokens with.
  - If set but not present in TRAZR_JWT_SECRETS, token issuance fails.
- TRAZR_JWT_ISSUER (optional)
  - Sets `iss` on new tokens; if provided, validation enforces it.
- TRAZR_JWT_AUDIENCE (optional)
  - Sets `aud` on new tokens; if provided, validation enforces it.
- TRAZR_JWT_TTL (optional; default 24h)
  - JWT lifetime for user login tokens (e.g., `24h`, `7d`).

### API Keys (service-to-service)
- TRAZR_API_KEY_PEPPER (required)
  - Server-side pepper for HMAC-SHA256 of API keys. The process will exit if not set.
- TRAZR_API_KEY_PEPPER_OLD (optional)
  - Previous pepper to allow rotation. When set, both peppers validate; new keys use the primary pepper.
- Key format (required): `trz-<8 hex>_<32 hex>`
  - All new keys are generated in this format and validation requires the `trz-` prefix.

### Passwords (end-user)
- TRAZR_PASSWORD_PEPPER (recommended)
  - Server-side password pepper appended before bcrypt hashing.
  - If unset, the package falls back to PASSWORD_PEPPER (legacy). Consider setting only TRAZR_PASSWORD_PEPPER.
- BCRYPT_COST (optional; default 12)
  - Cost parameter for bcrypt. Higher is slower and more secure; ensure acceptable latency.

### OAuth (social login)
- GitHub:
  - GITHUB_CLIENT_ID (required for GitHub OAuth)
  - GITHUB_CLIENT_SECRET (required)
  - GITHUB_REDIRECT_URL (required)
  - SESSION_SECRET (required)
    - Used to secure temporary cookies (state and PKCE verifier) during OAuth. Keep strong and secret.
- Google:
  - GOOGLE_CLIENT_ID (required for Google OAuth)
  - GOOGLE_CLIENT_SECRET (required)
  - GOOGLE_REDIRECT_URL (required)
  - Call InitGoogleOIDC() at startup.

### Optional operational flags
- TRAZR_JWT_M2M_TTL (optional; default 15m)
  - Lifetime for machine-to-machine JWTs issued via API key exchange.

### Recommended operational practices
- Store secrets in a secret manager (not in plain env files committed to VCS).
- Rotate TRAZR_JWT_SECRET and TRAZR_API_KEY_PEPPER regularly.
- Prefer setting both TRAZR_JWT_ISSUER and TRAZR_JWT_AUDIENCE in production.
- Enforce TLS everywhere.
- Rate limit sensitive endpoints (login, API key exchange).

### Examples

#### 1) JWT with rotation (multiple kids)
Example .env:
```env
# Primary fallback secret (used when no kid is set on tokens)
TRAZR_JWT_SECRET=superlongrandomfallbacksecret

# Map of kid -> secret used for signing/verification
TRAZR_JWT_SECRETS=v1=supersecret-v1,v2=supersecret-v2

# Sign all newly-issued tokens with kid "v2"
TRAZR_JWT_KID=v2

# Optional issuer/audience checks
TRAZR_JWT_ISSUER=trazr.example
TRAZR_JWT_AUDIENCE=trazr-clients

# User-session token TTL
TRAZR_JWT_TTL=24h
```
Notes:
- If TRAZR_JWT_KID is set, it must exist in TRAZR_JWT_SECRETS or issuance will fail.
- During rotation, set TRAZR_JWT_KID to the new kid, keep the old kid in TRAZR_JWT_SECRETS for validation, then remove it after all old tokens expire.

#### 2) API key pepper rotation
```env
# New pepper (required)
TRAZR_API_KEY_PEPPER=pepper-new-2025-08
# Old pepper (optional) — keeps validating older keys until decommissioned
TRAZR_API_KEY_PEPPER_OLD=pepper-old-2025-07
```
Key format returned to callers (example):
```
trz-beb1a737_812ffb78a05df70eaddcfdf6e300e8d6
```

#### 3) Password hashing configuration
```env
TRAZR_PASSWORD_PEPPER=pepper-for-passwords
BCRYPT_COST=12
```

#### 4) OAuth configuration (GitHub and Google)
```env
# GitHub
GITHUB_CLIENT_ID=...
GITHUB_CLIENT_SECRET=...
GITHUB_REDIRECT_URL=https://app.example.com/auth/github/callback
SESSION_SECRET=longrandomsessionsecret

# Google
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GOOGLE_REDIRECT_URL=https://app.example.com/auth/google/callback
```

- Store secrets in a secret manager (not in plain env files committed to VCS).
- Rotate TRAZR_JWT_SECRET and TRAZR_API_KEY_PEPPER regularly.
- Prefer setting both TRAZR_JWT_ISSUER and TRAZR_JWT_AUDIENCE in production.
- Enforce TLS everywhere.
- Rate limit sensitive endpoints (login, API key exchange).

---

## Features Implemented

- **User Registration & Login**
  - Secure password hashing (bcrypt)
  - Email normalization and validation
  - Account lockout after repeated failed logins
  - Flexible user profile fields and metadata (JSONB)

- **JWT Authentication**
  - Issuance and validation of JWTs for stateless sessions
  - Optional JWT blacklist helpers for consumers that explicitly use denylist validation. Collector route middleware currently validates signatures, claims, and expiry; password reset leaves previously issued backend JWTs valid until expiry.

- **Password Reset & Email Verification**
  - Secure, expiring tokens for password reset and email verification
  - Token usage tracking and expiry enforcement

- **Role-Based Access Control (RBAC)**
  - Assign, remove, and check user roles
  - List all roles for a user

- **Multi-Factor Authentication (MFA)**
  - TOTP-based MFA (compatible with Google Authenticator, Authy, etc.)
  - Enable/disable MFA per user

- **Audit Logging**
  - Logs all key authentication events (login, password change, etc.)

- **OAuth (Social Login) Support**
  - Modular endpoints for GitHub and Google OAuth login
  - User linking via `user_identities` table
  - Automatic user creation or linking on first OAuth login

- **Table Management**
  - `Ensure*Table` functions for all required tables
  - One-call `EnsureAllAuthTables` for schema setup

- **Comprehensive Unit Tests**
  - Table-driven tests for all business logic and edge cases
  - Optional real PostgreSQL/SMTP checks described in [development](../../docs/development.md)

## Next Steps: Integrating GitHub & Google OAuth

### 1. **Database Migration**
Add the `user_identities` table to your database:
```sql
CREATE TABLE user_identities (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL,
    provider_user_id VARCHAR(128) NOT NULL,
    email VARCHAR(255),
    UNIQUE(provider, provider_user_id)
);
```

### 2. **Configuration**
Set the following environment variables:
- For **GitHub**:
  - `GITHUB_CLIENT_ID`
  - `GITHUB_CLIENT_SECRET`
  - `GITHUB_REDIRECT_URL`
- For **Google**:
  - `GOOGLE_CLIENT_ID`
  - `GOOGLE_CLIENT_SECRET`
  - `GOOGLE_REDIRECT_URL`
- For sessions:
  - `SESSION_SECRET`

### 3. **App Initialization**
- For Google OAuth, call `InitGoogleOIDC()` at application startup.

### 4. **HTTP Routing**
Wire up the following handlers in your HTTP server:
- `/auth/github/login` → `GitHubLoginHandler`
- `/auth/github/callback` → `GitHubCallbackHandler(db, jwtIssuer)`
- `/auth/google/login` → `GoogleLoginHandler`
- `/auth/google/callback` → `GoogleCallbackHandler(db, jwtIssuer)`

Where `jwtIssuer` is a function that issues your JWTs (e.g., `GenerateJWT`).

### 5. **Frontend Integration**
- Add "Sign in with GitHub" and "Sign in with Google" buttons that hit the respective `/auth/*/login` endpoints.
- On successful callback, handle the returned JWT (store in localStorage/cookie, update app state, etc.).

### 6. **Security & Best Practices**
- Use HTTPS for all OAuth flows.
- Securely store all secrets and client credentials.
- Implement CSRF protection for OAuth state parameter in production.
- Validate and sanitize all user input.

### 7. **Testing**
- Add integration/E2E tests for OAuth flows.
- Use mocks for external providers in unit tests.

---

## Extending Further
- Add support for more OAuth providers (GitLab, Microsoft, etc.)
- Expand email notifications beyond the configured SMTP password-reset flow
- Enforce session revocation consistently across Collector routes

---

Use the maintained [configuration](../../docs/configuration.md) and [maintenance tasks](../../TASKS.md) for deployment requirements and current limits.

For questions or contributions, see [contributing](../../CONTRIBUTING.md) or contact the maintainers.
