# Goosie Makefile

# Variables
BINARY_NAME=goosie
BUILD_DIR=bin
TEST_OUTPUT_DIR=testdata
E2E_TEST_DIR=test/e2e
GO_FILES=$(shell find . -name '*.go' -not -path "./vendor/*")

# Build flags
LDFLAGS=-ldflags "-s -w"

.PHONY: all build clean install-playwright test generate-test-data

all: build

# Build the project binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/browser
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Clean build artifacts and test data
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -rf $(TEST_OUTPUT_DIR)
	@rm -f screenshot.png
	@go clean

# Install Playwright browsers and dependencies
install-playwright:
	@echo "Installing Playwright dependencies..."
	go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps

# Generate test data using the CLI tool
generate-test-data:
	@echo "Generating test data..."
	@mkdir -p $(TEST_OUTPUT_DIR)
	go run ./cmd/test-gen -output $(TEST_OUTPUT_DIR)

# Generate screenshots for all testdata HTML files
screenshots: generate-test-data
	@echo "Generating screenshots..."
	@mkdir -p $(TEST_OUTPUT_DIR)/screenshots
	HEADLESS=true go run ./cmd/screenshot-all -input $(TEST_OUTPUT_DIR) -output $(TEST_OUTPUT_DIR)/screenshots
	@echo "Screenshots saved to $(TEST_OUTPUT_DIR)/screenshots"

# Run tests
test: clean generate-test-data
	@echo "Running tests..."
	# Verify output.html exists
	@ls -l $(TEST_OUTPUT_DIR)/test_*.html | head -n 5
	
	# Run unit tests
	@echo "Running unit tests..."
	go test -v ./internal/... ./cmd/...
	
	# Run E2E tests with Playwright
	@echo "Running E2E tests..."
	go test -v ./$(E2E_TEST_DIR)

# Update test snapshots
update-snapshots: clean generate-test-data
	@echo "Updating test snapshots..."
	UPDATE_SNAPSHOTS=true go test -v ./$(E2E_TEST_DIR)
