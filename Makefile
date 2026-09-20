BINARY  := terraform-provider-deevnet
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# Terraform's filesystem mirror: the version directory carries no leading "v",
# and the platform directory must match the machine terraform runs on.
MIRROR_VERSION := $(patsubst v%,%,$(VERSION))
MIRROR_OS      := $(shell go env GOOS)
MIRROR_ARCH    := $(shell go env GOARCH)
MIRROR_DIR     := $(HOME)/.terraform.d/plugins/registry.terraform.io/deevnet/deevnet/$(MIRROR_VERSION)/$(MIRROR_OS)_$(MIRROR_ARCH)

.PHONY: default help build test testacc vet fmt docs clean mirror

default: help

help:
	@echo "Targets:"
	@echo "  build    build $(BINARY)"
	@echo "  test     unit tests"
	@echo "  testacc  acceptance tests (needs TF_ACC=1 and a Deevnet API; builds real objects)"
	@echo "  vet      go vet ./..."
	@echo "  fmt      gofmt -w ."
	@echo "  docs     regenerate docs/ from the schemas"
	@echo "  mirror   install into the local filesystem mirror terraform reads"
	@echo ""
	@echo "VERSION=$(VERSION)"

build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY)

# Install into the local filesystem mirror, which is how tenants get this
# provider: it is not published to a registry (ADR-0012 §7 keeps the source
# address deevnet/deevnet from day one, but nothing is served from there).
#
# A dirty or untagged tree is refused for the same reason `stage` refuses one
# in the API repository: a tenant resolving "0.2.0" has to get the same binary
# every time, and "v0.1.0-3-gabc1234-dirty" is not a version a constraint can
# match anyway.
mirror: build
	@case "$(MIRROR_VERSION)" in \
	  *-dirty|*-g*|dev) \
	    echo "refusing to mirror VERSION=$(VERSION); commit and tag first" >&2; exit 1;; \
	esac
	install -d $(MIRROR_DIR)
	install -m 0755 $(BINARY) $(MIRROR_DIR)/$(BINARY)
	@echo "mirrored $(MIRROR_DIR)/$(BINARY)"

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
