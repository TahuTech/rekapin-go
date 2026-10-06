# Variabel bisa dioverride: make build GOOS=linux GOARCH=amd64
TAILWIND_VERSION ?= v4.1.14
TAILWIND := bin/tailwindcss
GOOS ?= linux
GOARCH ?= amd64

.PHONY: css css-watch build run test migrate tools clean \
	dev dev-down dev-reset db-up admin psql logs test-docker build-docker

# Dipakai compose.yaml agar container berjalan sebagai user host.
export UID := $(shell id -u)
export GID := $(shell id -g)
COMPOSE := docker compose

$(TAILWIND):
	mkdir -p bin
	curl -fsSL -o $(TAILWIND) https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-linux-x64
	chmod +x $(TAILWIND)

tools: $(TAILWIND)

css: $(TAILWIND)
	$(TAILWIND) -i web/tailwind/input.css -o web/static/app.css --minify

css-watch: $(TAILWIND)
	$(TAILWIND) -i web/tailwind/input.css -o web/static/app.css --watch

# Binary statis, template & asset sudah ter-embed.
build: css
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o bin/rekapin ./cmd/rekapin

run: css
	go run ./cmd/rekapin serve

migrate:
	go run ./cmd/rekapin migrate

# Integration test butuh TEST_DATABASE_URL (database kosong khusus test).
test:
	go vet ./...
	go test ./...

clean:
	rm -f bin/rekapin

# ---------- Development via Docker (tanpa install Go/Node di host) ----------

dev: ## db + app dengan hot reload -> http://localhost:8080
	$(COMPOSE) up --build

dev-down:
	$(COMPOSE) down

dev-reset: ## hapus container + data DB dev
	$(COMPOSE) down -v

db-up: ## Postgres saja (untuk mode native: go run ./cmd/rekapin serve)
	$(COMPOSE) up -d --wait db

admin: ## buat/reset master dev: admin / admin12345
	printf 'admin12345\n' | $(COMPOSE) exec -T app go run ./cmd/rekapin user create -role master admin Administrator

psql:
	$(COMPOSE) exec db psql -U rekapin rekapin

logs:
	$(COMPOSE) logs -f app

test-docker:
	$(COMPOSE) run --rm --no-deps app sh -c 'go vet ./... && go test ./...'

# Binary produksi linux/amd64 dibangun di container (host tidak perlu Go).
build-docker:
	$(COMPOSE) run --rm --no-deps app sh -c 'tailwindcss -i web/tailwind/input.css -o web/static/app.css --minify && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/rekapin ./cmd/rekapin'
