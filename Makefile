.PHONY: build clean cov docker format help integrationtest lint proto setup-test-env sqlc teardown-test-env test vet

GOLANGCI_LINT ?= $(shell \
	echo "docker run --rm -v $$(pwd):/app -w /app golangci/golangci-lint:v2.9.0 golangci-lint"; \
)

## build: build bancod and banco binaries
build:
	@echo "Building bancod..."
	@go build -o bancod ./cmd/bancod/
	@echo "Building banco CLI..."
	@go build -o banco ./cmd/banco/

## proto: generate protobuf code
proto:
	@echo "Generating protobuf code..."
	@cd api-spec && buf generate

## sqlc: generate sqlc code
sqlc:
	@echo "Generating sqlc code..."
	@cd internal/infrastructure/db/sqlite/sqlc && sqlc generate

## docker: build production Docker image
docker:
	@echo "Building Docker image..."
	@docker build -t bancod .

## clean: cleans build artifacts
clean:
	@echo "Cleaning..."
	@go clean
	@rm -f bancod banco

## cov: generates coverage report
cov:
	@echo "Coverage..."
	@go test -cover ./...

## help: prints this help message
help:
	@echo "Usage: \n"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

## format: rewrite Go files in-place using gofmt + goimports
format:
	@echo "Formatting code..."
	@$(GOLANGCI_LINT) fmt

## lint: lint codebase
lint:
	@echo "Linting code..."
	@$(GOLANGCI_LINT) run --fix --tests=false

## test: runs all tests
test:
	@echo "Running all tests..."
	@go test -v -race --count=1 ./...

## setup-test-env: start nigiri + arkd stack for integration tests
setup-test-env:
	@echo "Starting nigiri..."
	@nigiri start
	@echo "Starting arkd stack..."
	@docker compose -f test/docker-compose.yml up -d --build
	@echo "Waiting for services..."
	@sleep 15
	@echo "Creating arkd wallet..."
	@docker exec bancod-arkd arkd wallet create --password password || true
	@docker exec bancod-arkd arkd wallet unlock --password password || true
	@echo "Funding arkd..."
	@for i in 1 2 3; do nigiri faucet $$(docker exec bancod-arkd arkd wallet address | tr -d '[:space:]') 1; done
	@sleep 5
	@echo "Test environment ready."

## teardown-test-env: stop arkd stack + nigiri
teardown-test-env:
	@echo "Stopping arkd stack..."
	@docker compose -f test/docker-compose.yml down -v --remove-orphans
	@echo "Stopping nigiri..."
	@nigiri stop --delete

## integrationtest: run integration tests (requires setup-test-env)
integrationtest:
	@echo "Running integration tests..."
	@go test -v -count=1 -timeout=10m -race -p=1 -tags e2e ./test/e2e/...

## vet: code analysis
vet:
	@echo "Running code analysis..."
	@go vet ./...
