# terraform-provider-deevnet

The Terraform provider for the [Deevnet API](https://github.com/deevnet/deevnet-provisioning-api):
a tenant declares itself, its workloads and its published names, and holds no substrate credential
([ADR-0015](https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/0015-tenant-onboarding-through-api/)).

```hcl
terraform {
  required_providers {
    deevnet = {
      source  = "deevnet/deevnet"
      version = "~> 0.1"
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
  ssh_keys  = [file("~/.ssh/id_ed25519.pub")]
}

resource "deevnet_dns_record" "alias" {
  tenant  = deevnet_tenant.this.name
  name    = "api"
  address = deevnet_workload.web.address
}
```

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

## Restore instead of recreate

The framework's advice for a remote object that is gone is to drop it from state and let the next
plan create a new one. This provider does not, for the secret-bearing resources: a new tenant would
mean new keys, and a new workload would mean new addressing.

Instead `present` goes `false` when the API no longer holds the object, which makes the next plan an
update, and the update sends the index and secrets back from state. The API keeps that index when it
is free, and issues a new one only when another tenant took it (ADR-0012 §5, ADR-0015 §5). That is a
deliberate exception, confined to these resources.

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
