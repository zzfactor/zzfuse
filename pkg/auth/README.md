# Authentication

This package provides database-backed JWT sessions, explicit schema grants, roles, API keys, password resets, private SMTP delivery, and OAuth account linking. Runtime configuration is documented in [configuration](../../docs/configuration.md).

## Session authorization

`ValidateJWT` checks cryptographic claims only. HTTP authorization must use `ValidateSessionJWT(ctx, db, token)` or the existing request middleware with `WithSessionDatabase(ctx, db)` attached by the owning server instance. Missing database configuration, database errors, deleted or disabled accounts, revoked JTIs, and mismatched account versions fail closed. Session validation has a five-second bound within the request context.

Every validation reloads the account's current identity, roles, and schema grants. Token-supplied roles, grants, tenant identity, and user-session scopes cannot override that state. Empty grants deny nonadmins. Administrators retain access to all schemas. Grants use exact canonical lowercase PostgreSQL schema names, without wildcards or case folding.

Use `IssueSessionJWT` after a trusted authentication operation, `RenewSessionJWT` for renewal, and `RevokeSessionJWT` for logout. `GenerateJWT*` remain low-level compatibility/signing helpers and produce legacy tokens without the required account version. They cannot authorize Collector requests.

The extension supplies its instance-owned database in request context. Long-lived connections must periodically validate their session and permissions; an HTTP handshake alone does not authorize a connection forever.

## Migration and revocation

Normal auth initialization adds `auth_users.auth_version`, `disabled`, and `tenant_id`, plus `auth_user_schema_grants`. Initialization is additive and idempotent. Existing identities and roles remain; implicit schema access is not backfilled. Versionless JWTs issued before this migration require a fresh login or API-key exchange.

Password updates increment the account version. One-time reset-token consumption, password update, and version increment commit in the same transaction. A failed update consumes no token and revokes no session. Concurrent use of one reset token has one winner. Password changes compare the previously verified hash so they cannot overwrite a concurrently completed reset.

Explicit all-session revocation and account disable/re-enable also increment the version. Re-enabling an account does not revive its old JWTs. Role and grant changes take effect through current database authorization without relying on stale claims. Deleting an API key invalidates JWTs exchanged from that key on their next validation.

## Administrative access

Server routes should use these actor-checked functions, with the actor derived from a validated session:

- `GetUserAccess`: read current account authorization.
- `SetUserSchemaGrants`: atomically replace explicit grants.
- `SetUserRoles`: replace validated roles.
- `SetUserDisabled`: disable or re-enable an account.
- `RevokeUserSessions`: invalidate all previously issued account JWTs.
- `DeleteUserAsAdmin`: delete an account with last-administrator protection.

Nonadmins cannot self-grant or self-promote. Administrative mutations serialize with first-user registration and preserve the last enabled administrator. First registration on an empty installation retains the existing bootstrap policy, with exactly one administrator selected transactionally. Registration metadata does not assign roles or grants. Low-level storage/role helpers are trusted internal operations, not public authorization boundaries.

## API keys

API-key JWTs carry `api_key_id`. Validation reloads current key ownership, expiry, organization/project scope, and operation scopes, together with current account grants. They do not inherit administrator roles. Key creation accepts telemetry and notification read/write scope labels; nonadmins cannot select arbitrary organization/project IDs. Invalid scope labels fail closed.

The owning server enforces method-specific key scopes and keeps machine tokens out of account, administrator, and configuration operations. Schema grants and operation scopes both apply. Machine tokens must use API-key exchange again rather than user-session renewal.

## Password reset and OAuth

Private SMTP reset delivery uses the configured fixed frontend URL with the token only in its browser fragment. Requests use a bounded asynchronous dispatcher and a generic account-independent response. No tokens are returned in public responses or logged. Without secure delivery configuration, reset requests remain unavailable. See [configuration](../../docs/configuration.md) for SMTP and URL settings.

OAuth callbacks issue database-backed sessions, retaining their old callback signatures for source compatibility while ignoring legacy JWT-issuer callbacks. Google requires a verified email. GitHub account linking always uses a verified primary email. The provisioned table is `auth_user_identities`. Initialization copies historical `auth_identities` mappings if present, preserves that table, and rejects conflicting owners for operator resolution.

## Validation

Run module-local unit checks with `go test -mod=readonly -race ./...`. External integration checks are opt-in:

- `TestSessionMigrationRevocationAndAccessPostgres` and `TestConcurrentRegistrationAndLastAdminPostgres`: `TRAZR_IT=1`, `TEST_AUTH_SESSION_DSN` pointing at a disposable service with database-creation privileges. Each run creates and removes a unique database.
- `TestPasswordResetSMTPAndAtomicPostgres`: `TRAZR_IT=1`, `TEST_AUTH_RESET_DSN` pointing at a disposable PostgreSQL database. It uses a unique schema and local SMTP capture.

The full Collector/browser/SMTP authorization flows are covered by `make stack-e2e`. See [development](../../docs/development.md) for current commands and [maintenance tasks](../../TASKS.md) for remaining work.
