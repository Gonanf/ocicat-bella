# Ocicat Bella — recetas comunes (bun + go + docker)
# Backend vive en worktree aparte: ../ocicat-bella-backend/backend
set positional-arguments

# variables
backend_dir := "backend"
frontend_dir := "frontend"

# default: listar recetas
default:
    @just --list

# instalar deps de frontend
install:
    cd {{frontend_dir}} && bun install

# dev: astro + go en paralelo (ctrl-c corta ambos)
dev:
    #!/usr/bin/env bash
    set -e
    trap 'kill 0' EXIT
    (cd {{frontend_dir}} && bun run dev) &
    (cd {{backend_dir}} && go run ./cmd/api...) &
    wait

# build de producción
build:
    cd {{frontend_dir}} && bun run build
    cd {{backend_dir}} && realpath . && go build ./...

# test de todo
test:
    cd {{frontend_dir}} && bun run build >/dev/null   # smoke: que compile el sitio
    cd {{backend_dir}} && go test ./...

# lint/format check de todo
lint:
    cd {{frontend_dir}} && bunx astro check
    cd {{backend_dir}} && go vet ./...
    cd {{backend_dir}} && test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

# docker compose up -d
up:
    docker compose -f docker/docker-compose.yml up -d

# docker compose down
down:
    docker compose -f docker/docker-compose.yml down

# logs de compose
logs:
    docker compose -f docker/docker-compose.yml logs -f --tail=100

# limpiar builds locales
clean:
    rm -rf {{frontend_dir}}/dist
    cd {{backend_dir}} && go clean -cache -testcache 2>/dev/null || true
