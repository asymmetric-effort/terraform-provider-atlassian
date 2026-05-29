BINARY_NAME := terraform-provider-atlassian
BUILD_DIR := build
VERSION_FILE := VERSION
VERSION := $(shell cat $(VERSION_FILE))
GOFLAGS := -trimpath
LDFLAGS := -s -w -X main.version=$(VERSION)

MOCK_IMAGE := atlassian-mock-api
MOCK_CONTAINER := atlassian-mock-api

.PHONY: clean build lint test cover release release/patch release/minor release/major \
        api/build api/start api/stop

## ---- Build targets (Issue #3) ----

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@mkdir -p $(BUILD_DIR)
	@echo "Cleaning Go build cache..."
	@go clean -cache
	@echo "Stopping and removing mock API containers..."
	@docker stop $(MOCK_CONTAINER) 2>/dev/null || true
	@docker rm $(MOCK_CONTAINER) 2>/dev/null || true
	@docker rmi $(MOCK_IMAGE) 2>/dev/null || true
	@echo "Clean complete."

build: clean
	@echo "Building $(BINARY_NAME) v$(VERSION)..."
	@go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

lint:
	@echo "Running linters..."
	@echo "  gofmt..."
	@UNFORMATTED=$$(gofmt -l . | grep -v vendor/ || true); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "ERROR: Files not formatted with gofmt:"; \
		echo "$$UNFORMATTED"; \
		exit 1; \
	fi
	@echo "  go vet..."
	@go vet ./...
	@echo "  govulncheck..."
	@govulncheck ./...
	@echo "  jsonlint..."
	@JSONFILES=$$(find . -name '*.json' -not -path './vendor/*' -not -path './.git/*' -not -path './build/*' 2>/dev/null); \
	if [ -n "$$JSONFILES" ]; then \
		for f in $$JSONFILES; do \
			jsonlint -q "$$f" || exit 1; \
		done; \
	fi
	@echo "  yamllint..."
	@YAMLFILES=$$(find . -name '*.yml' -o -name '*.yaml' | grep -v vendor/ | grep -v '.git/' | grep -v 'build/' 2>/dev/null); \
	if [ -n "$$YAMLFILES" ]; then \
		yamllint -c .yamllint.yml $$YAMLFILES || exit 1; \
	fi
	@echo "  markdownlint..."
	@MDFILES=$$(find . -name '*.md' -not -path './vendor/*' -not -path './.git/*' -not -path './build/*' 2>/dev/null); \
	if [ -n "$$MDFILES" ]; then \
		markdownlint $$MDFILES || exit 1; \
	fi
	@echo "Lint complete."

## ---- Test targets (Issue #4) ----

test:
	@echo "Running test suite..."
	@echo ""
	@echo "=== Unit tests ==="
	@go test ./test/unit/... -v -count=1
	@echo ""
	@echo "=== Integration tests ==="
	@go test ./test/integration/... -v -count=1
	@echo ""
	@echo "=== End-to-end tests ==="
	@if [ -n "$$(find test/e2e -name '*_test.go' 2>/dev/null)" ]; then \
		go test ./test/e2e/... -v -count=1; \
	else \
		echo "  No e2e tests yet."; \
	fi
	@echo ""
	@echo "=== PDV tests ==="
	@echo "  No PDV tests yet."
	@echo ""
	@echo "Test suite complete."

cover:
	@echo "Running coverage analysis..."
	@go test ./test/unit/... ./test/integration/... -coverprofile=coverage.out -covermode=atomic -coverpkg=./internal/...
	@go tool cover -html=coverage.out -o coverage.html 2>/dev/null || true
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Coverage: $${COVERAGE}%"; \
	echo "Coverage report: coverage.out, coverage.html"; \
	if [ $$(awk "BEGIN {print ($$COVERAGE < 98.0)}") -eq 1 ]; then \
		echo "ERROR: Coverage $${COVERAGE}% is below the required 98% threshold."; \
		exit 1; \
	fi

## ---- Release targets (Issue #5) ----

release: release/patch

release/patch:
	@echo "Bumping patch version..."
	@IFS='.' read -r MAJOR MINOR PATCH < $(VERSION_FILE); \
	PATCH=$$((PATCH + 1)); \
	echo "$$MAJOR.$$MINOR.$$PATCH" > $(VERSION_FILE); \
	NEW_VERSION=$$(cat $(VERSION_FILE)); \
	echo "Version: v$$NEW_VERSION"; \
	git add $(VERSION_FILE); \
	git commit -m "chore: bump version to v$$NEW_VERSION"; \
	git tag -a "v$$NEW_VERSION" -m "Release v$$NEW_VERSION"; \
	echo "Tagged v$$NEW_VERSION (local only — not pushed)"

release/minor:
	@echo "Bumping minor version..."
	@IFS='.' read -r MAJOR MINOR PATCH < $(VERSION_FILE); \
	MINOR=$$((MINOR + 1)); \
	echo "$$MAJOR.$$MINOR.0" > $(VERSION_FILE); \
	NEW_VERSION=$$(cat $(VERSION_FILE)); \
	echo "Version: v$$NEW_VERSION"; \
	git add $(VERSION_FILE); \
	git commit -m "chore: bump version to v$$NEW_VERSION"; \
	git tag -a "v$$NEW_VERSION" -m "Release v$$NEW_VERSION"; \
	echo "Tagged v$$NEW_VERSION (local only — not pushed)"

release/major:
	@echo "Bumping major version..."
	@IFS='.' read -r MAJOR MINOR PATCH < $(VERSION_FILE); \
	MAJOR=$$((MAJOR + 1)); \
	echo "$$MAJOR.0.0" > $(VERSION_FILE); \
	NEW_VERSION=$$(cat $(VERSION_FILE)); \
	echo "Version: v$$NEW_VERSION"; \
	git add $(VERSION_FILE); \
	git commit -m "chore: bump version to v$$NEW_VERSION"; \
	git tag -a "v$$NEW_VERSION" -m "Release v$$NEW_VERSION"; \
	echo "Tagged v$$NEW_VERSION (local only — not pushed)"

## ---- Mock API targets (Issue #12) ----

api/build:
	@echo "Building mock API Docker image..."
	@docker build -t $(MOCK_IMAGE) -f internal/mock/Dockerfile .
	@echo "Mock API image built: $(MOCK_IMAGE)"

api/start:
	@echo "Starting mock API..."
	@docker stop $(MOCK_CONTAINER) 2>/dev/null || true
	@docker rm $(MOCK_CONTAINER) 2>/dev/null || true
	@docker run -d --name $(MOCK_CONTAINER) -p 8080:8080 $(MOCK_IMAGE)
	@echo "Mock API running on http://localhost:8080"

api/stop:
	@echo "Stopping mock API..."
	@docker stop $(MOCK_CONTAINER) 2>/dev/null || true
	@docker rm $(MOCK_CONTAINER) 2>/dev/null || true
	@echo "Mock API stopped."
