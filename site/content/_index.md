---
title: "Deevnet Terraform Provider"
type: docs
---

<div class="landing-hero">

# Deevnet Terraform Provider

<p class="subtitle"><code>deevnet/deevnet</code> {{< param version >}}</p>

</div>

The `deevnet/deevnet` provider is how a tenant builds itself on a
[Deevnet](https://deevnet.github.io/deevnet-docs/) site: its network, its workloads, its
published names, and the Wi-Fi keys, registry entries, addresses and broker accounts its devices
need. It talks to the [Deevnet API](https://deevnet.github.io/deevnet-provisioning-api/) with a
single token, and a tenant holds no other credential.

<div class="section-cards">
<a class="section-card" href="docs/guides/install/">
<h3>Install</h3>
<p>Put the provider where <code>terraform init</code> finds it.</p>
</a>
<a class="section-card" href="docs/provider/">
<h3>Provider</h3>
<p>The provider block and its three arguments.</p>
</a>
<a class="section-card" href="docs/resources/">
<h3>Resources</h3>
<p>Every argument and attribute, generated from the schemas.</p>
</a>
<a class="section-card" href="docs/guides/restore/">
<h3>Restore Instead of Recreate</h3>
<p>What the provider does when the API no longer holds an object.</p>
</a>
</div>

## A whole tenant

```terraform
terraform {
  required_providers {
    deevnet = {
      source  = "deevnet/deevnet"
      version = "~> 0.6"
    }
  }
}

provider "deevnet" {}

resource "deevnet_tenant" "this" {
  name = "tdemo"
}

resource "deevnet_workload" "web" {
  tenant   = deevnet_tenant.this.name
  name     = "web"
  ssh_keys = [file("~/.ssh/id_ed25519.pub")]
}

resource "deevnet_iot_wifi_key" "devices" {
  tenant      = deevnet_tenant.this.name
  name        = "devices"
  trust_class = "iot"
}
```

The tenant guide in the Deevnet docs walks through a first apply:
[Getting Started](https://deevnet.github.io/deevnet-docs/docs/runbook/tenant/getting-started/).
