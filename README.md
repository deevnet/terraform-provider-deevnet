# terraform-provider-deevnet

The Terraform provider for the [Deevnet API](https://github.com/deevnet/deevnet-provisioning-api):
a tenant declares itself, its workloads and its published names, and holds no substrate credential
([ADR-0015](https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/tenant-model/0015-tenant-onboarding-through-api/)).

```hcl
terraform {
  required_providers {
    deevnet = {
      source  = "deevnet/deevnet"
      version = "~> 0.6"
    }
  }
}

provider "deevnet" {
  # endpoint, token and ca_certificate default to DEEVNET_API_ENDPOINT,
  # DEEVNET_API_TOKEN and DEEVNET_API_CACERT.
}

resource "deevnet_tenant" "this" {
  name = "tdemo"
}

resource "deevnet_workload" "web" {
  tenant    = deevnet_tenant.this.name
  name      = "web"
  cores     = 2
  memory_mb = 2048
  ssh_keys  = [file("~/.ssh/id_ed25519.pub")] # the public half; the private key never leaves you
}

output "login" {
  value = "ssh ${deevnet_workload.web.login_user}@${deevnet_workload.web.fqdn}"
}

resource "deevnet_dns_record" "alias" {
  tenant  = deevnet_tenant.this.name
  name    = "api"
  address = deevnet_workload.web.address
}

resource "deevnet_iot_wifi_key" "devices" {
  tenant      = deevnet_tenant.this.name
  name        = "devices"
  trust_class = "iot"
}

resource "deevnet_iot_device" "stand" {
  tenant      = deevnet_tenant.this.name
  name        = "stand-1"
  trust_class = "iot"
}

resource "deevnet_iot_broker_account" "stand" {
  tenant = deevnet_tenant.this.name
  name   = "stand-1"
  device = deevnet_iot_device.stand.name

  # Relative to the tenant. The API writes the "tdemo/" prefix itself, which is
  # what confines a tenant to its own topics.
  publish   = ["lightstand/+/telemetry"]
  subscribe = ["lightstand/+/command"]
}

output "wifi" {
  # Flash devices with these. Never hardcode the SSID: the same trust class is
  # a different SSID at another site.
  value     = {
    ssid = deevnet_iot_wifi_key.devices.ssid
    psk  = deevnet_iot_wifi_key.devices.psk
  }
  sensitive = true
}

output "broker" {
  value = {
    username = deevnet_iot_broker_account.stand.username
    password = deevnet_iot_broker_account.stand.password
    # What the broker actually enforces, prefix and all.
    publish = deevnet_iot_broker_account.stand.granted_publish
  }
  sensitive = true
}
```

## Documentation

**https://deevnet.github.io/terraform-provider-deevnet/** is the reference: every resource's
arguments and attributes, generated from the schemas, and three guides.

| | |
|---|---|
| [Install](https://deevnet.github.io/terraform-provider-deevnet/docs/guides/install/) | the provider is not in the public registry; three ways into Terraform's local mirror |
| [Provider](https://deevnet.github.io/terraform-provider-deevnet/docs/provider/) | the provider block and its arguments |
| [Resources](https://deevnet.github.io/terraform-provider-deevnet/docs/resources/) | the seven resources |
| [What a Tenant Holds](https://deevnet.github.io/terraform-provider-deevnet/docs/guides/tenant-credential/) | one token, and state as the authoritative copy of what the API issues |
| [Restore Instead of Recreate](https://deevnet.github.io/terraform-provider-deevnet/docs/guides/restore/) | why `present = false` leads to a restore, never a new object |
| [Release Notes](https://deevnet.github.io/terraform-provider-deevnet/docs/release-notes/) | what changed in each version; the source is `CHANGELOG.md` |

`docs/` is generated, in the layout the Terraform Registry reads: edit the schema descriptions,
`examples/` or `templates/`, then run `make docs`. CI fails when `docs/` is stale. `site/` is the
Hugo site that renders it for GitHub Pages.

## Development

```bash
make build     # the provider binary
make test      # unit tests
make testacc   # acceptance tests: they build real objects (see below)
make docs      # regenerate docs/ from the schemas, examples/ and templates/
make docs-check  # what CI runs: regenerate, validate, fail if docs/ changed
make site-serve  # the documentation site on localhost:1313
```

Acceptance tests need a Deevnet API and run only with `TF_ACC=1`:

```bash
TF_ACC=1 \
DEEVNET_API_ENDPOINT=https://api.mobile.deevnet.net \
DEEVNET_API_TOKEN=<operator token> \
DEEVNET_TEST_TENANT=tfacc \
make testacc
```

`DEEVNET_TEST_WORKLOADS=1` adds the workload test, which **builds and destroys a VM on the tenant
hypervisor**. `DEEVNET_TEST_TENANT` must never name a live tenant: the tests create and destroy it.

## Releases

Tags are `vMAJOR.MINOR.PATCH`. Every release has a section in `CHANGELOG.md`, written before the
tag: `make release` uses it as the release's notes and refuses to run without one. It builds `darwin`/`linux` × `amd64`/`arm64` zips with
`SHA256SUMS`, and publishes them with `install-provider.sh` and `tenant-check.sh` as a GitHub
release; `make stage` installs the same files into the Builder's tenant downloads tree.

The source address is `deevnet/deevnet` from day one (ADR-0012 §7), served from Terraform's local
mirror while the provider is not published, so publishing later changes nothing in a tenant's
`required_providers`. `.goreleaser.yaml` holds the signed, manifest-carrying release the public
registry requires; nothing runs it yet.
