# zzfuse frontend

Next.js dashboard for [zzfactor/zzfuse](https://github.com/zzfactor/zzfuse), with credentials authentication, user administration, and API documentation.

## Development

Use Node.js 24 LTS (or a supported runtime matching `package.json`), then run:

```sh
npm ci
cp .env.example .env.local
# Set NEXTAUTH_SECRET to a random secret and configure BACKEND_URL if needed.
npm run dev
```

Open http://localhost:3000. The default backend origin is http://localhost:18888.

## Validation

```sh
npm run lint
npm run typecheck
npm run build
npm audit --omit=dev
```

`npm start` serves the production build. Next.js 16 uses Turbopack and `src/proxy.ts`; linting is a separate ESLint CLI check. Both App Router pages and Pages Router API handlers live under `src/`.

## Backend contracts

Authentication proxies target `/api/v1/auth/`: registration sends `username`, `email`, and `password`; login sends the email identifier as `username`. Password reset requests use `request-password-reset`. The backend returns 503 until SMTP delivery is configured, then sends a private reset link and returns a generic public response. New links use a token fragment consumed in the browser, with reset-page caching and referrers disabled. Token resets send `token` and `new_password` in the body to `reset-password`. See [SMTP configuration](../docs/configuration.md#password-reset-delivery) and the [full-stack test](../docs/development.md#full-stack-checks).

User administration targets `/api/v1/admin/users` and `/api/v1/admin/users/{id}`. Proxies preserve the supplied Bearer authorization header; the backend enforces authorization. API documentation is loaded through `/api/backend-openapi`, using the configured backend origin.

The current Go extension exposes filtered signal queries through its individual `/api/v1/logs`, `/api/v1/spans`, `/api/v1/metrics`, `/api/v1/runs`, and `/api/v1/profiles` endpoints. It does not register the arbitrary SQL endpoint or vector search endpoint that the legacy dashboard expected. SQL and vector forms therefore show an unavailable message by default; authenticated POST requests return 503 rather than being sent to nonexistent routes.

`SQL_SEARCH_PATH` and `VECTOR_SEARCH_PATH` can opt in to separately implemented backend handlers with the legacy JSON contracts. Set only an absolute path on `BACKEND_URL`, such as `/custom/search`, after verifying that handler exists. Both handlers accept `{ query }`; vector search also forwards bounded `k`, `offset`, optional `filter`, and `namespace`. Successful responses must provide `{ results: [...] }`. These settings do not enable or add a Go handler.

## Dependency compatibility

Next.js and React are pinned together with the lockfile. TanStack Table stays on v8 to preserve its existing table API. TypeScript stays on the latest 5.x compiler compatible with the framework integration. ESLint stays on the latest 9.x release because Next's current React, import, and accessibility plugins declare peers through ESLint 9.

The production dependency audit is clean. The full audit currently reports development-only findings through Next's ESLint plugin and `fast-glob` / `micromatch` / `braces`; the registry has no patched `braces` release. Do not apply the audit's suggested framework-config downgrade without reviewing compatibility.

Upgrade guidance: [Next.js 16 migration](https://nextjs.org/docs/app/guides/upgrading/version-16) and [ESLint configuration](https://nextjs.org/docs/app/api-reference/config/eslint).
