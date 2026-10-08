---
page_title: "Install the provider"
title: "Install"
weight: 1
description: |-
  Put the provider into Terraform's local mirror, from a Deevnet site, from GitHub or from source.
---

# Install the provider

**The provider is not in the public Terraform Registry.** It goes into Terraform's implicit
local mirror, and `terraform init` finds it there:

```
~/.terraform.d/plugins/registry.terraform.io/deevnet/deevnet/<version>/<os>_<arch>/
```

The source address is `deevnet/deevnet` all the same, so a tenant's `required_providers`
block reads as it would for a registry provider.

## At a Deevnet site

**On the tenant developer network, the site's download server is the fastest route.** It also
installs the `grafana` provider, for dashboards as code.

```bash
curl -fsSLk -O https://downloads.mobile.deevnet.net:8443/deevnet-root-ca.pem
openssl x509 -in deevnet-root-ca.pem -noout -fingerprint -sha256
curl -fsSL --cacert deevnet-root-ca.pem -O https://downloads.mobile.deevnet.net:8443/scripts/install-provider.sh
bash install-provider.sh
```

The first fetch is the only unverified one, and the fingerprint is what you check: compare it
against the one in the tenant guide's
[Before You Start](https://deevnet.github.io/deevnet-docs/docs/runbook/tenant/getting-started/before-you-start/) page before trusting
anything the site serves.

## From GitHub

**Anywhere else, the same script installs from this repository's GitHub releases.**

```bash
curl -fsSL -O https://github.com/deevnet/terraform-provider-deevnet/releases/latest/download/install-provider.sh
bash install-provider.sh --github
```

## From source

**Building from a tag needs Go and make, and builds for this machine only.**

```bash
git clone https://github.com/deevnet/terraform-provider-deevnet
cd terraform-provider-deevnet
git checkout vX.Y.Z
make mirror
```

`make mirror` refuses an untagged or modified tree: a version constraint has to resolve to
the same binary every time.

## What the script checks

**Every zip is verified against `SHA256SUMS` before it is unpacked.** The site's download
server is verified against the Deevnet Root CA the script carries. The script needs only
`curl`, `unzip` and `shasum` or `sha256sum`, and runs on macOS and Linux, `amd64` and
`arm64`.

`tenant-check.sh`, published beside it, reports what else a tenant's computer still needs
and prints the install command for macOS (brew), Fedora (dnf) or Debian and Ubuntu (apt).
