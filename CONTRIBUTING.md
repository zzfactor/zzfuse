# Contributing

Use the tool versions in [development](docs/development.md). This repository has multiple Go modules; root-level `go test ./...` is not valid.

```sh
make go-check
make frontend-check
make python-test
make docs-check
```

Install frontend/Python dependencies first. Keep external-service tests opt-in and use disposable databases. Format changed code, test changed behavior, and keep pull requests focused.

Describe the problem, resulting behavior, checks, and integration limits. Update API contracts/documentation when behavior changes. New plans belong in `docs/plans/`; completed historical notes belong in `docs/archive/`.

Contributions use the [Apache 2.0 license](LICENSE). Retain third-party notices and report vulnerabilities privately through [SECURITY.md](SECURITY.md).
