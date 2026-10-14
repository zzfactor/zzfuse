SHELL := /bin/sh
.DEFAULT_GOAL := help

GO ?= go
PYTHON ?= services/.venv/bin/python
IMAGE ?= zzfactor/zzfuse:dev
GOARCH ?= $(shell $(GO) env GOARCH)
GO_MODULES := pkg/auth pkg/auth/client pkg/config pkg/event pkg/metrics pkg/repository pkg/util trazrexporter trazrextension trazrprocessor trazr-mcp trazr-otel

.PHONY: help build test vet go-check verify frontend-install frontend-dev frontend-check python-install python-test docs-check install-ocb generate docker profile-e2e database-image stack-images stack-e2e

help:
	@printf '%s\n' 'build: collector + MCP' 'go-check: verify, build, vet, and unit tests in every Go module' 'profile-e2e: OTLP + disposable TimescaleDB + authenticated queries (requires Docker)' 'stack-e2e: browser + SMTP + telemetry + NATS + real embeddings (requires Docker)' 'frontend-install / frontend-dev / frontend-check' 'python-install / python-test' 'docs-check: maintained documentation links' 'install-ocb / generate: Collector regeneration' 'docker: build a local Linux image'

build:
	cd trazr-otel && $(GO) build -mod=readonly -o trazr-otel .
	cd trazr-mcp && $(GO) build -mod=readonly -o trazr-mcp .

test:
	@set -e; for module in $(GO_MODULES); do printf '\nTesting %s\n' "$$module"; (cd "$$module" && $(GO) test -mod=readonly -timeout 90s ./...); done

vet:
	@set -e; for module in $(GO_MODULES); do printf '\nVetting %s\n' "$$module"; (cd "$$module" && $(GO) vet -mod=readonly ./...); done

go-check:
	@set -e; for module in $(GO_MODULES); do printf '\nChecking %s\n' "$$module"; (cd "$$module" && $(GO) mod verify && $(GO) build -mod=readonly ./... && $(GO) vet -mod=readonly ./... && $(GO) test -mod=readonly -timeout 90s ./...); done

verify: go-check frontend-check python-test docs-check

frontend-install:
	cd frontend && npm ci

frontend-dev:
	cd frontend && npm run dev

frontend-check:
	cd frontend && npm run lint && npm run typecheck && npm run build

python-install:
	uv venv services/.venv --python 3.11
	uv pip sync --index-strategy unsafe-best-match --python services/.venv/bin/python services/requirements.txt

python-test:
	cd services && $(abspath $(PYTHON)) -m unittest discover -s tests -v

docs-check:
	python3 scripts/check-doc-links.py

database-image:
	docker build -t zzfuse-database:local build/database

profile-e2e: build database-image
	python3 scripts/test-profile-e2e.py

stack-images: database-image
	docker build -t zzfuse-collector:release-readiness .
	docker build -t zzfuse-services:release-readiness --build-arg DOWNLOAD_MODELS=sentence-transformers/all-MiniLM-L6-v2 services
	docker build -f frontend/Dockerfile -t zzfuse-frontend:release-readiness .

stack-e2e: stack-images
	python3 scripts/test-stack-e2e.py

install-ocb:
	sh ./install-ocb-local.sh

generate:
	GOTOOLCHAIN=go1.27.2 ./ocb --config builder-config.yaml

docker:
	docker build -t $(IMAGE) .
