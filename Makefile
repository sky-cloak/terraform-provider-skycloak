VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build install test testacc lint docs tidy generate

build: ## Build the provider binary
	go build -ldflags "$(LDFLAGS)" -o bin/terraform-provider-skycloak

test: ## Unit tests
	go test ./... -timeout 120s

testacc: ## Acceptance tests (creates real resources; needs SKYCLOAK_API_KEY)
	TF_ACC=1 go test ./internal/provider/... -v -timeout 120m

lint: ## Lint (golangci-lint must be installed)
	golangci-lint run ./...

docs: ## Generate provider docs from schema + examples
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 generate --provider-name skycloak

tidy: ## Tidy modules
	go mod tidy

generate: ## Regenerate the API client from internal/apiclient/openapi.yaml
	go generate ./internal/apiclient/...
