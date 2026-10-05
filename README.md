# terraform-provider-deevnet

The Terraform provider for the [Deevnet API](https://github.com/deevnet/deevnet-provisioning-api):
a tenant declares itself, its workloads and its published names, and holds no substrate credential
([ADR-0015](https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/0015-tenant-onboarding-through-api/)).

```hcl
terraform {
  required_providers {
    deevnet = {
      source  = "deevnet/deevnet"
      version = "~> 0.4"
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

## Install

The provider is not in the public registry. It goes into Terraform's implicit local mirror
(`~/.terraform.d/plugins/registry.terraform.io/deevnet/deevnet/<version>/<os>_<arch>/`), and
`terraform init` finds it there. Three ways in, fastest first:

```bash
# At a Deevnet site, on DVNTM-TD: from the site's tenant downloads (also installs the
# grafana provider for dashboards as code). Check the site root's fingerprint against the
# tenant guide before trusting anything it signs.
curl -fsSLk -O https://downloads.mobile.deevnet.net:8443/deevnet-root-ca.pem
openssl x509 -in deevnet-root-ca.pem -noout -fingerprint -sha256
curl -fsSL --cacert deevnet-root-ca.pem -O https://downloads.mobile.deevnet.net:8443/scripts/install-provider.sh
bash install-provider.sh

# Anywhere: prebuilt from this repository's GitHub releases
bash install-provider.sh --github

# From source (needs Go and make): builds for this machine only
git checkout vX.Y.Z && make mirror
```

The script verifies every zip against `SHA256SUMS`, and the site's download server against
the site CA it carries. The only unverified fetch is the CA itself, and its fingerprint is what
you check. `tenant-check.sh` beside it says what
else a tenant laptop still needs, with the install command for macOS (brew), Fedora (dnf) or
Debian/Ubuntu (apt).

Maintainers: `make release` builds `darwin`/`linux` × `amd64`/`arm64` zips with `SHA256SUMS`
and publishes them as a GitHub release; `make stage` installs the same files into the Builder's
tenant downloads tree.

## What a tenant holds

One token. The substrate admits a tenant name and issues a **single-use enrollment token**,
delivered age-encrypted into the tenant's repository. The first apply spends it and receives the
tenant's own token, which every later call uses. Nothing else about the substrate reaches the
tenant: no Proxmox credential, no vault access.

The tenant's TSIG key, state-store credential and API token come back into Terraform state, which
is their authoritative copy (ADR-0015 §4). Keep that state where ADR-0007 says.

## Resources

| Resource | What it is |
|---|---|
| `deevnet_tenant` | the tenant: index, numbering, DNS zone and key, state-store credential, API token |
| `deevnet_workload` | a VM in the tenant's network; the API derives its VMID, MAC and address |
| `deevnet_dns_record` | a name in the tenant's zone, with its PTR |
| `deevnet_iot_wifi_key` | a Wi-Fi key for the tenant's IoT devices, bound to its trust class's VLAN |
| `deevnet_iot_device` | an entry in the tenant's device registry: an identity, carrying no credential |
| `deevnet_iot_broker_account` | an MQTT account on the platform broker, for a device or a workload |

## Restore instead of recreate

The framework's advice for a remote object that is gone is to drop it from state and let the next
plan create a new one. This provider does not, for the secret-bearing resources: a new tenant would
mean new keys, and a new workload would mean new addressing.

Instead `present` goes `false` when the API no longer holds the object, which makes the next plan an
update, and the update sends the index and secrets back from state. The API keeps that index when it
is free, and issues a new one only when another tenant took it (ADR-0012 §5, ADR-0015 §5). That is a
deliberate exception, confined to these resources.

**A Wi-Fi key is the sharpest case**, because the thing at the other end is a device in a wall. The
key in state is what its devices were flashed with, so a restore sends it back and the controller is
made to match them. `secrets_stored: false` triggers the same path, for a key the API still lists
but can no longer read.

The flip side is worth saying plainly: **`terraform apply -replace`, or removing and re-adding the
block, issues a NEW key, and every device flashed with the old one stops associating until it is
reflashed.** There is no guard against that, because revoking is sometimes exactly what you mean.

**A broker account is sharper still.** The API keeps only a bcrypt hash of the password, so unlike a
Wi-Fi key it cannot resupply one from its own records — this state is the only copy that exists.
Two things follow. A restore sends the password back from state, so the broker is made to match the
clients. And when a create fails at the last step, the provider keeps the account it was handed,
password included, rather than letting the apply error discard it; the account's `status` is not
`ready`, so the next plan retries it.

Topic patterns are written **relative to the tenant** — `lightstand/+/scene`, never
`eds/lightstand/+/scene` — and the API writes the prefix. `granted_publish` and `granted_subscribe`
report the absolute form the broker enforces. State records the relative form, so a grant that
changed on the broker shows up as a diff against the configuration and the next apply corrects it.

## Development

```bash
make build     # the provider binary
make test      # unit tests
make testacc   # acceptance tests: they build real objects (see below)
make docs      # regenerate docs/ from the schemas
```

Acceptance tests need a Deevnet API and run only with `TF_ACC=1`:

```bash
TF_ACC=1 \
DEEVNET_API_ENDPOINT=https://api.mobile.deevnet.net:8080 \
DEEVNET_API_TOKEN=<operator token> \
DEEVNET_TEST_TENANT=tfacc \
make testacc
```

`DEEVNET_TEST_WORKLOADS=1` adds the workload test, which **builds and destroys a VM on the tenant
hypervisor**. `DEEVNET_TEST_TENANT` must never name a live tenant: the tests create and destroy it.

## Releases

Tags are `vMAJOR.MINOR.PATCH`, and a release carries the manifest, `SHA256SUMS` and a GPG signature
the public registry requires, from the first tag (ADR-0012 §7). The source address is
`deevnet/deevnet` from day one, served from the site's filesystem mirror while the provider is not
yet published, so publishing later changes nothing in a tenant's `required_providers`.
