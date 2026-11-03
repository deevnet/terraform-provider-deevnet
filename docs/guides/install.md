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

On Windows the same tree is under `%APPDATA%\terraform.d\plugins\`. Under WSL2, Windows'
Linux, it is the Linux path, and the Linux steps below apply unchanged.

The source address is `deevnet/deevnet` all the same, so a tenant's `required_providers`
block reads as it would for a registry provider.

## At a Deevnet site

**On the tenant developer network, the site's download server is the fastest route.** It also
installs the `grafana` provider, for dashboards as code.

```bash
curl -fsSLk -O https://downloads.mobile.deevnet.net/deevnet-root-ca.pem
openssl x509 -in deevnet-root-ca.pem -noout -fingerprint -sha256
curl -fsSL --cacert deevnet-root-ca.pem -O https://downloads.mobile.deevnet.net/scripts/install-provider.sh
bash install-provider.sh
```

On Windows, in PowerShell:

```powershell
curl.exe -fsSLk -O https://downloads.mobile.deevnet.net/deevnet-root-ca.pem
$c = [Security.Cryptography.X509Certificates.X509Certificate2]::new("$PWD\deevnet-root-ca.pem")
[BitConverter]::ToString([Security.Cryptography.SHA256]::Create().ComputeHash($c.RawData))
curl.exe -fsSL --ssl-revoke-best-effort --cacert deevnet-root-ca.pem -O https://downloads.mobile.deevnet.net/scripts/install-provider.ps1
powershell -ExecutionPolicy Bypass -File install-provider.ps1
```

Write `curl.exe`, not `curl`: in Windows PowerShell 5.1, `curl` is another command.
`-ExecutionPolicy Bypass` runs this one script without changing the policy for others.

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

On Windows:

```powershell
curl.exe -fsSL -O https://github.com/deevnet/terraform-provider-deevnet/releases/latest/download/install-provider.ps1
powershell -ExecutionPolicy Bypass -File install-provider.ps1 -GitHub
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
`curl`, `unzip` and `shasum` or `sha256sum`, and runs on macOS, Linux and WSL2, `amd64` and
`arm64`. `install-provider.ps1` does the same on Windows with what Windows ships: PowerShell and
`curl.exe`.

`tenant-check.sh`, published beside it, reports what else a tenant's computer still needs
and prints the install command for macOS (brew), Fedora (dnf) or Debian and Ubuntu (apt); under
WSL2 it also checks WSL's networking. `tenant-check.ps1` does the same on Windows, with `winget`.
