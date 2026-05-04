BINARY   := sqmeter-ascom-alpaca
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"
MAIN     := ./cmd/$(BINARY)

CONFORM_VERSION  := v4.3.0
CONFORM_DIR      := .conform
CONFORM_LINUX_URL := https://github.com/ASCOMInitiative/ConformU/releases/download/$(CONFORM_VERSION)/conformu.linux-x64.tar.xz

.PHONY: all assets css build build-windows build-linux build-linux-arm64 test lint fmt vet clean run \
        service-install service-start service-stop service-uninstall service-status \
        conform conform-download

all: build

assets: css

css:
	npm run build:assets

build: assets
	go build $(LDFLAGS) -o bin/$(BINARY) $(MAIN)

build-windows: assets
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-windows-amd64.exe $(MAIN)

build-linux: assets
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-amd64 $(MAIN)

build-linux-arm64: assets
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-arm64 $(MAIN)

test:
	go test ./... -race -count=1

test-cover:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

fmt:
	gofmt -w .

lint:
	@echo "checking formatting..."
	@test -z "$$(gofmt -l .)" || (echo "files need formatting:"; gofmt -l .; exit 1)
	@echo "running vet..."
	go vet ./...

vet:
	go vet ./...

clean:
	rm -rf bin/ dist/ coverage.out coverage.html

run: build
	./bin/$(BINARY)

# Windows service management (run as Administrator)
service-install: build
	./bin/$(BINARY) --service install

service-start:
	./bin/$(BINARY) --service start

service-stop:
	./bin/$(BINARY) --service stop

service-uninstall:
	./bin/$(BINARY) --service uninstall

service-status:
	./bin/$(BINARY) --service status

# Download ConformU for Linux (CI / Linux dev machines).
# On macOS, install ConformU from https://github.com/ASCOMInitiative/ConformU/releases
conform-download:
	@mkdir -p $(CONFORM_DIR)
	curl -sSL "$(CONFORM_LINUX_URL)" | tar -xJ -C $(CONFORM_DIR)
	chmod +x $(CONFORM_DIR)/conformu

# Run ASCOM ConformU conformance test against the service + mock SQMeter.
# Set CONFORM_BIN to override the binary path (auto-detected otherwise).
conform: build
	bash scripts/conform.sh
