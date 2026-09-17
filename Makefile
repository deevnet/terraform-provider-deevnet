BINARY  := terraform-provider-deevnet
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: default help build test testacc vet fmt docs clean

default: help

help:
	@echo "Targets:"
	@echo "  build    build $(BINARY)"
	@echo "  test     unit tests"
	@echo "  testacc  acceptance tests (needs TF_ACC=1 and a Deevnet API; builds real objects)"
	@echo "  vet      go vet ./..."
	@echo "  fmt      gofmt -w ."
	@echo "  docs     regenerate docs/ from the schemas"
	@echo ""
	@echo "VERSION=$(VERSION)"

build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY)

test:
	go test ./...

# Acceptance tests drive real Terraform against a real API. TF_ACC is the
# switch; without the environment below they skip.
testacc:
	TF_ACC=1 go test ./internal/provider/ -v -timeout 30m

vet:
	go vet ./...

fmt:
	gofmt -w .

# tfplugindocs renders docs/ from the schemas; the registry publishes them.
docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest generate --provider-name deevnet

clean:
	rm -f $(BINARY)
