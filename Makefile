# Goosie Makefile
BINARY_NAME=goosie
BUILD_DIR=bin
GO_FILES=$(shell find . -name '*.go' -not -path "./vendor/*" -not -path "./_v1_old/*")

.PHONY: all build build-small macos-app clean test test-race e2e bench gate wpt wpt-pilot wpt-unit

all: build

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -trimpath -ldflags "-s -w" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/goosie
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

build-small:
	@echo "Building $(BINARY_NAME) (size optimized)..."
	@mkdir -p $(BUILD_DIR)
	go build -trimpath -ldflags "-s -w -buildid=" -o $(BUILD_DIR)/$(BINARY_NAME)-small ./cmd/goosie
	@echo "Size-optimized build complete: $(BUILD_DIR)/$(BINARY_NAME)-small"

macos-app:
	@test "$$(go env GOOS)" = "darwin" || (echo "macos-app requires macOS"; exit 1)
	@mkdir -p $(BUILD_DIR)/Goosie.app/Contents/MacOS
	go build -trimpath -ldflags "-s -w" -o $(BUILD_DIR)/Goosie.app/Contents/MacOS/$(BINARY_NAME) ./cmd/goosie
	cp cmd/goosie/macos/Info.plist $(BUILD_DIR)/Goosie.app/Contents/Info.plist
	@echo "Packaged native app: $(BUILD_DIR)/Goosie.app"

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@go clean

test:
	@echo "Running tests..."
	go test -count=1 ./...

test-race:
	@echo "Running tests with race detector..."
	go test -race ./...

e2e:
	@echo "Running the headless browser journey suite..."
	go test -count=1 -run '^TestE2E' ./cmd/goosie

bench:
	@echo "Running benchmarks..."
	go run ./cmd/goosie -bench -frames 2000

gate:
	@echo "Running gate suite..."
	go run ./cmd/goosie -gate -scene checkerboard -frames 600 -out /tmp/gate.json

wpt-unit:
	@echo "Running WPT unit tests..."
	go test -count=1 -run 'TestWPTCurator|TestWPTServer|TestWPTImageCompare|TestWPTConfigDefaults|TestWPTConfigLoad|TestWPTMinPassRateFloor|TestWPTSuiteThreshold|TestWPTCheckoutReusable|TestShouldSkip|TestWPTPilotDiscovery' -v ./test/wpt/

wpt-pilot: build
	@echo "Running WPT pilot (local reftests)..."
	go test -count=1 -timeout 10m -run TestWPTPilot -v ./test/wpt/

parity: build
	@echo "Scoring render parity vs Chromium (fail-closed)..."
	python3 testdata/parity.py --render --render-chromium --binary ./bin/goosie

wpt: build
	@echo "Running WPT suite..."
	bash scripts/run-wpt.sh --download
