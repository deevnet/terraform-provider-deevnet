# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`terraform-provider-deevnet` is the `deevnet/deevnet` provider: the interface tenants use to build
themselves through the Deevnet API (ADR-0015 in `deevnet-docs`). It is public, and the source
address is `deevnet/deevnet` from day one (ADR-0012 §7).

## Commands

```bash
make build     # the provider binary
make test      # unit tests
make testacc   # acceptance tests; needs TF_ACC=1 and a Deevnet API
make docs      # regenerate docs/ from the schemas
```

## Rules that are easy to get wrong

- **Never RemoveResource for a secret-bearing resource.** A tenant or workload the API no longer
  holds keeps its state and sets `present = false`, so the next apply restores it with the index and
  secrets it already has (ADR-0012 §5, ADR-0015 §5). Recreating would mint new keys and new
  addressing.
- **The API issues; the provider records.** Index, VMID, MAC, address and every secret are computed
  attributes with `UseStateForUnknown`, so a plan never proposes changing them.
- **A tenant holds one credential.** The provider takes a single bearer token. Don't add substrate
  credentials (Proxmox, vault, PowerDNS) to the schema.
- **Secrets are `Sensitive: true`**, and error messages come from the API, which never puts a secret
  or a backend address in one.
- **Acceptance tests build real objects.** They need `TF_ACC=1` and are gated on
  `DEEVNET_TEST_TENANT`, which must never name a live tenant. `DEEVNET_TEST_WORKLOADS=1` builds a VM
  on the tenant hypervisor; `DEEVNET_TEST_BROKER=1` writes accounts to the platform broker.
- **Broker account patterns are relative in config and absolute in the API's answer.** State records
  the relative form - stripping the prefix back off - so a plan stays empty AND a grant that drifted
  on the broker shows as a diff. `granted_publish`/`granted_subscribe` carry the absolute form.
- **A failed create can still carry a secret.** The API returns `502` with the account and its
  password when its own record was written and the writer step was not, and the API keeps only a
  hash. `client.APIError` therefore keeps the raw body, and the resource sets state from the partial
  object before returning the error - the framework persists state set on a failed Create.
