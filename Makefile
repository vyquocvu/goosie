# Goosie Makefile
BINARY_NAME=goosie
BUILD_DIR=bin
TEST_OUTPUT_DIR=testdata
E2E_TEST_DIR=test/e2e
GO_FILES=$(shell find . -name '*.go' -not -path "./vendor/*")

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILDTIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Build flags
VERSION_LDFLAGS=-X github.com/vyquocvu/goosie/internal/version.Version=$(VERSION) -X github.com/vyquocvu/goosie/internal/version.Commit=$(COMMIT) -X github.com/vyquocvu/goosie/internal/version.BuildTime=$(BUILDTIME)
LDFLAGS=-ldflags "-s -w $(VERSION_LDFLAGS)"

# Reproducible build flags for release builds
REPRODUCIBLE_FLAGS=-trimpath -ldflags "-s -w -buildid= -X github.com/vyquocvu/goosie/internal/version.Version=$(VERSION) -X github.com/vyquocvu/goosie/internal/version.Commit=$(COMMIT) -X github.com/vyquocvu/goosie/internal/version.BuildTime=reproducible"

# Size-optimized release flags. UPX compression is optional because it can slow
# startup slightly and may be rejected by some antivirus scanners.
SMALL_FLAGS=-trimpath -ldflags "-s -w -buildid= $(VERSION_LDFLAGS)"
UPX ?= upx
UPX_FLAGS ?= --best --lzma

# Headless browser variant for URL screenshots via Fyne test driver
HEADLESS_FLAGS=-tags headless -trimpath -ldflags "-s -w $(VERSION_LDFLAGS)"

.PHONY: all build build-small build-small-upx build-reproducible build-headless clean install-playwright test generate-test-data smoke-test

all: build

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/browser
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

build-small:
	@echo "Building $(BINARY_NAME) (size optimized)..."
	@mkdir -p $(BUILD_DIR)
	go build $(SMALL_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-small ./cmd/browser
	@echo "Size-optimized build complete: $(BUILD_DIR)/$(BINARY_NAME)-small"

build-small-upx: build-small
	@command -v $(UPX) >/dev/null 2>&1 || (echo "UPX not found; install upx or set UPX=/path/to/upx"; exit 1)
	$(UPX) $(UPX_FLAGS) $(BUILD_DIR)/$(BINARY_NAME)-small
	@echo "UPX-compressed build complete: $(BUILD_DIR)/$(BINARY_NAME)-small"

build-reproducible:
	@echo "Building $(BINARY_NAME) (reproducible)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(REPRODUCIBLE_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/browser
	@echo "Reproducible build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

build-headless:
	@echo "Building $(BINARY_NAME) (headless)..."
	@mkdir -p $(BUILD_DIR)
	go build $(HEADLESS_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-headless ./cmd/browser
	@echo "Headless build complete: $(BUILD_DIR)/$(BINARY_NAME)-headless"

build-headless-cli:
	@echo "Building $(BINARY_NAME)-cli (standalone headless renderer)..."
	@mkdir -p $(BUILD_DIR)
	go build -trimpath -ldflags "-s -w -X github.com/vyquocvu/goosie/internal/version.Version=$(VERSION) -X github.com/vyquocvu/goosie/internal/version.Commit=$(COMMIT) -X github.com/vyquocvu/goosie/internal/version.BuildTime=$(BUILDTIME)" -o $(BUILD_DIR)/$(BINARY_NAME)-cli ./cmd/headless
	@echo "CLI build complete: $(BUILD_DIR)/$(BINARY_NAME)-cli"

smoke-test: build
	@echo "Running smoke tests..."
	@$(BUILD_DIR)/$(BINARY_NAME) -version 2>&1 | grep -q "commit" || (echo "FAIL: version check"; exit 1)
	@echo "PASS: version check"
	@# Test headless mode starts and renders
	@echo '<html><body><p>smoke</p></body></html>' | go run ./cmd/headless -output /tmp/goosie-smoke.png 2>&1 | grep -q PNG || (echo "FAIL: headless render"; exit 1)
	@echo "PASS: headless render"
	@rm -f /tmp/goosie-smoke.png
	@echo "All smoke tests passed."

# Clean build artifacts and test data
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -rf $(TEST_OUTPUT_DIR)
	@rm -f screenshot.png
	@go clean

install-playwright:
	@echo "Installing Playwright dependencies..."
	go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps

generate-test-data:
	@echo "Generating test data..."
	@mkdir -p $(TEST_OUTPUT_DIR)
	go run ./cmd/test-gen -output $(TEST_OUTPUT_DIR)

screenshots: generate-test-data
	@echo "Generating screenshots..."
	@mkdir -p $(TEST_OUTPUT_DIR)/screenshots
	HEADLESS=true go run ./cmd/screenshot-all -input $(TEST_OUTPUT_DIR) -output $(TEST_OUTPUT_DIR)/screenshots
	@echo "Screenshots saved to $(TEST_OUTPUT_DIR)/screenshots"

test: clean generate-test-data
	@echo "Running tests..."
	@ls -l $(TEST_OUTPUT_DIR)/test_*.html | head -n 5
	@echo "Running unit tests..."
	go test -v ./internal/... ./cmd/...
	@echo "Running E2E tests..."
	go test -v -tags=e2e ./$(E2E_TEST_DIR)

update-snapshots: clean generate-test-data
	@echo "Updating test snapshots..."
	UPDATE_SNAPSHOTS=true go test -v -tags=e2e ./$(E2E_TEST_DIR)
