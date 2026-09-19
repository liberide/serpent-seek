SHELL := /bin/sh
VERSION ?= 1.0.2
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/liberide/serpent-seek/internal/version.Version=$(VERSION) \
	-X github.com/liberide/serpent-seek/internal/version.Commit=$(COMMIT) \
	-X github.com/liberide/serpent-seek/internal/version.BuildDate=$(BUILD_DATE)

.PHONY: all build web dev test lint vet check license image up down up-extdb logs clean

all: build

## Build the SPA bundle into web/build
web:
	cd web && npm ci && npm run build

## Build the single Go binary (embeds web/build)
build: web
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o serpentseek ./cmd/serpentseek

## Run the backend against the Vite dev server
dev:
	cd web && npm run dev

## Run all Go tests
test:
	go test ./... -cover

## Static analysis
lint: vet
	golangci-lint run ./...

vet:
	go vet ./...

## Svelte/TypeScript checks
check:
	cd web && npm run check

## Dependency license audit (code dependencies only)
license:
	go run github.com/google/go-licenses@latest report ./... 2>/dev/null || \
		echo "go-licenses not available; see README for the audit command"
	cd web && npx license-checker --production --excludePrivatePackages --onlyAllow "MIT;Apache-2.0;BSD-2-Clause;BSD-3-Clause;ISC"

## Build the Docker image
image:
	docker build --build-arg TAG=$(VERSION) --build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) -t serpentseek:$(VERSION) .

## Compose helpers
up:
	docker compose up -d --build

down:
	docker compose down

up-extdb:
	docker compose --profile extdb up -d --build

logs:
	docker compose logs -f

clean:
	rm -rf serpentseek web/.svelte-kit
	find web/build -mindepth 1 ! -name .keep -exec rm -rf {} + 2>/dev/null || true
