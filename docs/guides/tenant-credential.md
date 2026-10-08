---
page_title: "What a tenant holds"
title: "What a Tenant Holds"
weight: 2
description: |-
  A tenant holds one token, and its Terraform state is the authoritative copy of everything the API issues.
---

# What a tenant holds

**A tenant holds one credential: its Deevnet token.** Nothing else about the substrate
reaches the tenant: no hypervisor credential, no vault access.

## Enrollment

**The first apply trades a single-use enrollment token for the tenant's own token.** The
substrate admits a tenant name and issues the enrollment token, delivered encrypted into the
tenant's repository. The first `terraform apply` spends it and receives the tenant's own
token, which every later call uses
([ADR-0015](https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/tenant-model/0015-tenant-onboarding-through-api/)).

Both are given to the provider the same way, as `DEEVNET_API_TOKEN`.

## State is the authoritative copy

**Everything the API issues comes back into Terraform state, and that state is its
authoritative copy.** That covers the tenant's DNS update key, its state-store credential, its
log tokens, its dashboard login, its Wi-Fi keys, its broker passwords and its API token. The
API keeps the token only as a hash.

Keep that state where
[ADR-0007](https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/tenant-model/0007-terraform-state-custody/) says. Losing
it loses the only readable copy of some of these secrets; see
[Restore Instead of Recreate](/docs/guides/restore/) for what the provider does with it.
