# A whole tenant: itself, a workload, and a name beside it.
#
# The tenant's only credential is its Deevnet token: the enrollment token the
# substrate issued at admission on the first apply, then its own token, which
# this state holds. There is no Proxmox credential and no vault access here.

terraform {
  required_version = ">= 1.5"

  required_providers {
    deevnet = {
      source  = "deevnet/deevnet"
      version = "~> 0.1"
    }
  }

  # The state store the substrate offers (ADR-0007). Its credential comes from
  # the tenant resource below, so it is filled in on the second init - or the
  # block is deleted, and the tenant keeps its own custody.
  #
  # backend "s3" { ... }
}

provider "deevnet" {
  # DEEVNET_API_ENDPOINT, DEEVNET_API_TOKEN, DEEVNET_API_CACERT
}

resource "deevnet_tenant" "this" {
  name = "tdemo"
}

resource "deevnet_workload" "app" {
  tenant    = deevnet_tenant.this.name
  name      = "app"
  cores     = 2
  memory_mb = 2048
  ssh_keys  = var.ssh_keys
}

resource "deevnet_dns_record" "service" {
  tenant  = deevnet_tenant.this.name
  name    = "service"
  address = deevnet_workload.app.address
}

variable "ssh_keys" {
  type        = list(string)
  description = "Public keys for the workloads' cloud-init account."
  default     = []
}

output "subnet" { value = deevnet_tenant.this.subnet }
output "app_address" { value = deevnet_workload.app.address }
output "app_fqdn" { value = deevnet_workload.app.fqdn }

# What the substrate issued this tenant. Sensitive: the state is their
# authoritative copy (ADR-0015 §4).
output "tsig_secret" {
  value     = deevnet_tenant.this.tsig_secret
  sensitive = true
}

output "state_credentials" {
  value = {
    endpoint   = deevnet_tenant.this.state_endpoint
    bucket     = deevnet_tenant.this.state_bucket
    key_prefix = deevnet_tenant.this.state_key_prefix
    access_key = deevnet_tenant.this.state_access_key
    secret_key = deevnet_tenant.this.state_secret_key
  }
  sensitive = true
}
