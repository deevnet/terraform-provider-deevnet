---
page_title: "Restore instead of recreate"
title: "Restore Instead of Recreate"
weight: 3
description: |-
  When the API no longer holds an object, the provider restores it from state rather than creating a new one.
---

# Restore instead of recreate

**When the API no longer holds an object, the provider restores it from state instead of
creating a new one.** The usual advice for a remote object that is gone is to drop it from
state and let the next plan create another. For the resources that carry secrets or addressing
that would be a loss: a new tenant would mean new keys, and a new workload would mean a new
address.

## The present attribute

**`present` goes `false` when the API no longer holds the object, which makes the next plan
an update.** The update sends the index and secrets back from state. The API keeps that index
when it is free, and issues a new one only when another tenant took it
([ADR-0012](https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/tenant-model/0012-iot-platform-api/) §5,
[ADR-0015](https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/tenant-model/0015-tenant-onboarding-through-api/) §5).

`secrets_stored: false` triggers the same path, for an object the API still lists but whose
secrets it can no longer read.

## Wi-Fi keys

**A restore sends the key back, and the wireless controller is made to match the devices.**
The key in state is what the tenant's devices were flashed with, and those devices cannot be
asked to change.

The flip side: `terraform apply -replace`, or removing and re-adding the block, issues a
**new** key, and every device flashed with the old one stops associating until it is
reflashed. Nothing guards against that, because revoking is sometimes exactly what is meant.

## Broker accounts

**The state holds the only copy of a broker password.** The API keeps a bcrypt hash, so
unlike a Wi-Fi key it cannot resupply one from its own records. Two things follow:

- A restore sends the password back from state, so the broker is made to match the clients.
- When a create fails at its last step, the provider keeps the account it was handed,
  password included, rather than letting the error discard it. The account's `status` is not
  `ready`, so the next plan retries it.

Topic patterns are written **relative to the tenant**: `lightstand/+/scene`, never
`tdemo/lightstand/+/scene`. The API writes the prefix. `granted_publish` and
`granted_subscribe` report the absolute form the broker enforces. State records the relative
form, so a grant that changed on the broker shows as a diff and the next apply corrects it.

## Device addresses

**A device address is restored the same way, though it is no secret.** The device network is
shared by every tenant, so the API allocates the address rather than deriving it, and the
state is what remembers which one. A restore asks for the same address again. Dropping the
resource from state would let a rebuilt API hand the device whichever address was lowest at
that moment.
