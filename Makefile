BINARY  := terraform-provider-deevnet
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# Terraform's filesystem mirror: the version directory carries no leading "v",
# and the platform directory must match the machine terraform runs on.
MIRROR_VERSION := $(patsubst v%,%,$(VERSION))
MIRROR_OS      := $(shell go env GOOS)
MIRROR_ARCH    := $(shell go env GOARCH)
MIRROR_DIR     := $(HOME)/.terraform.d/plugins/registry.terraform.io/deevnet/deevnet/$(MIRROR_VERSION)/$(MIRROR_OS)_$(MIRROR_ARCH)

# Prebuilt releases (CHG-0025): the platforms a tenant's laptop runs, zipped
# the way a registry would serve them, with SHA256SUMS. `release` publishes
# them on GitHub (the off-site route); `stage` puts them in the Builder's
# tenant downloads tree, which the observability store serves to DVNTM-TD.
PLATFORMS      := darwin_arm64 darwin_amd64 linux_amd64 linux_arm64
DIST           := dist
GRAFANA_VERSION := 4.46.0
ARTIFACTS_ROOT ?= /srv/deevnet-http
STAGE_DIR      := $(ARTIFACTS_ROOT)/tenant

.PHONY: default help build test testacc vet fmt docs docs-check site site-serve clean mirror release-build release stage

default: help

help:
	@echo "Targets:"
	@echo "  build    build $(BINARY)"
	@echo "  test     unit tests"
	@echo "  testacc  acceptance tests (needs TF_ACC=1 and a Deevnet API; builds real objects)"
	@echo "  vet      go vet ./..."
	@echo "  fmt      gofmt -w ."
	@echo "  docs     regenerate docs/ from the schemas, examples/ and templates/"
	@echo "  docs-check  fail if docs/ is stale or does not validate"
	@echo "  site     build the documentation site into site/public"
	@echo "  site-serve  serve the documentation site on localhost:1313"
	@echo "  mirror   install into the local filesystem mirror terraform reads"
	@echo "  release-build  zips for $(PLATFORMS), SHA256SUMS and the two scripts, in $(DIST)/"
	@echo "  release  release-build, then a GitHub release with them attached"
	@echo "  stage    release-build, then install them under $(STAGE_DIR) (sudo)"
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

# Refuses what mirror refuses: a release is a version a constraint can match.
release-build:
	@case "$(MIRROR_VERSION)" in \
	  *-dirty|*-g*|dev) \
	    echo "refusing to release VERSION=$(VERSION); commit and tag first" >&2; exit 1;; \
	esac
	rm -rf $(DIST) && mkdir -p $(DIST)/scripts
	@for p in $(PLATFORMS); do \
	  os=$${p%_*}; arch=$${p#*_}; bin=$(DIST)/$$p/$(BINARY)_v$(MIRROR_VERSION); \
	  mkdir -p $(DIST)/$$p; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $$bin . || exit 1; \
	  (cd $(DIST)/$$p && zip -q ../$(BINARY)_$(MIRROR_VERSION)_$$p.zip $(BINARY)_v$(MIRROR_VERSION)) || exit 1; \
	  rm -rf $(DIST)/$$p; \
	done
	cd $(DIST) && sha256sum *.zip > SHA256SUMS
	@for f in install-provider.sh tenant-check.sh; do \
	  sed -e 's/@VERSION@/$(MIRROR_VERSION)/g' -e 's/@GRAFANA@/$(GRAFANA_VERSION)/g' scripts/$$f > $(DIST)/scripts/$$f; \
	  chmod 0755 $(DIST)/scripts/$$f; \
	done
	@ls -1 $(DIST) $(DIST)/scripts

release: release-build
	gh release create v$(MIRROR_VERSION) $(DIST)/*.zip $(DIST)/SHA256SUMS $(DIST)/scripts/*.sh \
	  --title "v$(MIRROR_VERSION)" --notes "Prebuilt for $(PLATFORMS). Install: bash install-provider.sh --github"

# The Builder's tenant downloads tree. install-provider.sh reads
# provider/<version>/ and scripts/ from it.
stage: release-build
	sudo install -d -o nginx -g nginx -m 0755 $(STAGE_DIR)/provider/$(MIRROR_VERSION) $(STAGE_DIR)/scripts
	sudo install -o nginx -g nginx -m 0644 $(DIST)/*.zip $(DIST)/SHA256SUMS $(STAGE_DIR)/provider/$(MIRROR_VERSION)/
	sudo install -o nginx -g nginx -m 0755 $(DIST)/scripts/*.sh $(STAGE_DIR)/scripts/
	@echo "staged $(STAGE_DIR)/provider/$(MIRROR_VERSION) and $(STAGE_DIR)/scripts"

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

# tfplugindocs renders docs/ from the schemas, examples/ and templates/, in the
# layout the registry reads. Its version is pinned in tools/go.mod. The
# documentation site (site/) renders the same files.
TFPLUGINDOCS := cd tools && go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs

docs:
	$(TFPLUGINDOCS) generate --provider-dir .. --provider-name deevnet

# What CI runs: a schema or example changed without `make docs` fails here.
docs-check: docs
	$(TFPLUGINDOCS) validate --provider-dir .. --provider-name deevnet
	@test -z "$$(git status --porcelain -- docs)" || { echo "docs/ is stale: run 'make docs' and commit the result" >&2; git status --short -- docs; exit 1; }

site:
	HUGO_PARAMS_VERSION=$(VERSION) hugo --source site --gc --minify

site-serve:
	HUGO_PARAMS_VERSION=$(VERSION) hugo server --source site --bind 0.0.0.0 --baseURL http://localhost:1313/terraform-provider-deevnet/

clean:
	rm -rf $(BINARY) $(DIST) site/public site/resources
