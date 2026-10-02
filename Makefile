GO ?= go
GOFLAGS := -mod=vendor
export GOFLAGS
export GOWORK := off
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE := $(shell git show -s --format=%cI HEAD 2>/dev/null)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

.PHONY: build test race lint fmt fmt-check run smoke scripts-test fuzz bench performance release release-smoke
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/folderwatch ./cmd/folderwatch
test:
	$(GO) test ./...
race:
	$(GO) test -race ./...
lint: fmt-check
	python3 scripts/vendor_guard.py
	python3 scripts/check_core_boundary.py
	$(GO) vet ./...
fmt:
	gofmt -w cmd internal tools gui/backend gui/host gui/main.go vendor/github.com/fsnotify/fsnotify/backend_kqueue.go
fmt-check:
	@test -z "$$(gofmt -l cmd internal tools gui/backend gui/host gui/main.go vendor/github.com/fsnotify/fsnotify/backend_kqueue.go)" || (gofmt -l cmd internal tools gui/backend gui/host gui/main.go vendor/github.com/fsnotify/fsnotify/backend_kqueue.go; exit 1)
run:
	$(GO) run ./cmd/folderwatch $(ARGS)
smoke: build
	python3 scripts/smoke.py bin/folderwatch
	python3 scripts/watch_smoke.py bin/folderwatch
	python3 scripts/semantic_smoke.py bin/folderwatch
	python3 scripts/tui_smoke.py bin/folderwatch
scripts-test:
	python3 -m unittest discover -s scripts -p 'test_*.py' -v
fuzz:
	python3 scripts/fuzz.py
bench:
	$(GO) test ./internal/snapshot ./internal/tui -run='^$$' -bench=. -benchmem -benchtime=1x
performance:
	python3 scripts/performance.py
release:
	python3 scripts/release.py --version $(VERSION)
release-smoke:
	python3 scripts/release_smoke.py dist/$(VERSION)

# One module/vendor tree: desktop builds must retain the reviewed native patch.
WAILS := $(CURDIR)/bin/wails
.PHONY: gui-tools gui-setup gui-bindings gui-check gui-build gui-dev gui-e2e
gui-tools:
	@test "$$('$(WAILS)' version 2>/dev/null | head -n 1)" = "v2.10.1" || GOBIN='$(CURDIR)/bin' GOFLAGS= $(GO) install github.com/wailsapp/wails/v2/cmd/wails@v2.10.1
gui-setup: gui-tools
	cd gui/frontend && npm ci --no-audit --no-fund
gui-bindings: gui-tools
	@mkdir -p gui/frontend/dist
	@test -f gui/frontend/dist/index.html || printf '<!doctype html><title>Binding generation only</title>\n' > gui/frontend/dist/index.html
	cd gui && '$(WAILS)' generate module -tags desktop
gui-check:
	cd gui/frontend && npm run check && npm test && npm run build
gui-build: gui-bindings gui-check
	python3 scripts/vendor_guard.py
	cd gui && '$(WAILS)' build -clean -m -nosyncgomod -tags desktop -ldflags '$(LDFLAGS)'
gui-dev: gui-bindings
	cd gui && '$(WAILS)' dev -m -nosyncgomod -tags desktop -devserver 127.0.0.1:34115
gui-e2e:
	cd gui/frontend && npm run test:e2e
