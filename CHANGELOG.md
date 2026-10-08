# Release Notes

Every release of the `deevnet/deevnet` provider, newest first. Versions are
`MAJOR.MINOR.PATCH`; until 1.0 a minor release may change behavior, and the notes say so
under **Upgrading**.

A resource needs an API that serves it. Each release names the
[Deevnet API](https://deevnet.github.io/deevnet-provisioning-api/docs/release-notes/) version its
new resources and attributes need.

## Unreleased

### Added

- A documentation site, https://deevnet.github.io/terraform-provider-deevnet/. The resource
  reference is generated from the schemas, and CI fails when it is stale.
- A description on every attribute. The `tenant` argument had none on five resources.
- An example for the provider block and for each resource, under `examples/`.

## 0.6.0 (2026-10-06)

Needs API v0.10.0 for `deevnet_iot_address`.

### Added

- **`deevnet_iot_address`**: a fixed address on the device network for a registered device, and
  the device's name in the tenant's zone (ADR-0035, CHG-0044). The API picks the address and
  the state remembers it, so a restore asks for the same one.

### Changed

- **The scripts trust the Deevnet Root CA** (CHG-0033). `install-provider.sh` and
  `tenant-check.sh` carry it, and the file a tenant downloads is `deevnet-root-ca.pem`.
- `install-provider.sh` says what it is downloading.

### Upgrading

- **The trust anchor has a new name.** Replace `site-ca.pem` or `deevnet-mobile-root-ca.pem`
  with `deevnet-root-ca.pem`, and point `DEEVNET_API_CACERT` at it.

## 0.5.0 (2026-09-27)

Needs API v0.9.0.

### Added

- **`deevnet_workload.login_user`**: the account a workload's `ssh_keys` landed on, so an output
  can print `ssh <login_user>@<fqdn>` (CHG-0028).
- **`deevnet_iot_wifi_key.mac`**: binds a key to one client. It suits the tenant developer
  network's `tenant_dev` trust class, one key per computer (CHG-0029).

### Changed

- `tenant-check.sh` verifies the state store over TLS (CHG-0030).

## 0.4.1 (2026-09-24)

### Added

- **Prebuilt releases** for macOS and Linux on `amd64` and `arm64`, with `SHA256SUMS`
  (CHG-0025). A tenant no longer needs Go to install the provider.
- **`install-provider.sh`**, which installs the provider into Terraform's local mirror from the
  site's download server or from GitHub, and **`tenant-check.sh`**, which reports what a
  tenant's computer still needs.

## 0.4.0 (2026-09-24)

Needs API v0.8.0.

### Added

- **Dashboard login on `deevnet_tenant`**: `dashboard_url`, `dashboard_org_id`,
  `dashboard_username` and `dashboard_password` (CHG-0024). They feed the `grafana` provider, so
  a tenant keeps its dashboards as code.

## 0.3.0 (2026-09-22)

Needs API v0.6.0.

### Added

- **Log store access on `deevnet_tenant`**: `log_endpoint`, `log_account_id`,
  `log_select_header`, `log_ingest_token` and `log_read_token`.
- `make mirror`, which installs a build into the local mirror Terraform reads.

## 0.2.0 (2026-09-20)

The first tagged release. Needs API v0.5.0 for broker accounts.

### Added

- **`deevnet_tenant`, `deevnet_workload` and `deevnet_dns_record`**: a tenant, its VMs and its
  published names.
- **`deevnet_iot_wifi_key`**: a Wi-Fi key for a tenant's devices, per trust class.
- **`deevnet_iot_device`**: an entry in the tenant's device registry.
- **`deevnet_iot_broker_account`**: an MQTT account for a device or a workload.
- **Restore instead of recreate.** A resource the API no longer holds sets `present = false`
  and is restored from state on the next apply, with the index and secrets it already had.
- **Resupplying secrets.** When the API can no longer read the secrets it holds,
  `secrets_stored` goes `false` and the next apply sends them back from state.
- **Enrollment in one apply.** The first apply spends the enrollment token and continues with
  the tenant's own token.
