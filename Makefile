APP     := proxy-privacy
OUTDIR  := build
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags="-s -w -X main.commit=$(COMMIT) -X main.date=$(DATE)"
INSTALL_DIR := /usr/local/bin

DETECTED_OS   := $(shell uname -s | tr A-Z a-z)
DETECTED_ARCH := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

.PHONY: all test build build-linux build-darwin build-darwin-arm64 build-windows install clean

all: test build

test:
	go test ./... -count=1

build: build-linux build-darwin build-darwin-arm64 build-windows

install: build
	BIN="$(OUTDIR)/$(APP)-$(DETECTED_OS)-$(DETECTED_ARCH)"; \
	case "$(DETECTED_OS)" in windows*) BIN="$$BIN.exe";; esac; \
	echo "  Installing $$BIN -> $(INSTALL_DIR)/$(APP)"; \
	sudo cp "$$BIN" "$(INSTALL_DIR)/$(APP)"

build-linux:
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $(OUTDIR)/$(APP)-linux-amd64     ./cmd/$(APP)/

build-darwin:
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o $(OUTDIR)/$(APP)-darwin-amd64    ./cmd/$(APP)/

build-darwin-arm64:
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o $(OUTDIR)/$(APP)-darwin-arm64    ./cmd/$(APP)/

build-windows:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(OUTDIR)/$(APP)-windows-amd64.exe ./cmd/$(APP)/

clean:
	rm -f $(OUTDIR)/$(APP) \
	      $(OUTDIR)/$(APP)-linux-amd64 \
	      $(OUTDIR)/$(APP)-darwin-amd64 \
	      $(OUTDIR)/$(APP)-darwin-arm64 \
	      $(OUTDIR)/$(APP)-windows-amd64.exe
