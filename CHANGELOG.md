# Release Notes

Every release of the `deevnet/deevnet` provider, newest first. Versions are
`MAJOR.MINOR.PATCH`; until 1.0 a minor release may change behavior, and the notes say so
under **Upgrading**.

A resource needs an API that serves it. Each release names the
[Deevnet API](https://deevnet.github.io/deevnet-provisioning-api/docs/release-notes/) version its
new resources and attributes need.

## Unreleased

### Added

- `deevnet_alert_rule_group`: a group of alert rules over the tenant's metrics or its logs
  (ADR-0023 §5, CHG-0071). Each runs in the tenant's own vmalert with its log read token, and its
  alerts reach the ntfy topic `<tenant>-<topic>` (`ntfy_topic`). A rule vmalert won't load, or one
  that sets the `tenant` label, fails the apply with the API's reason. Needs Deevnet API v0.25.0
  and a site with alerting.
- `deevnet_tenant` exports `notify_endpoint`, `notify_username`, `notify_topic_prefix` and the
  sensitive `notify_password`: the ntfy login the tenant's phone uses, on site, to receive its
  alerts. Empty at a site without alerting.

### Upgrading

- A tenant created before its site had alerting gets `notify_password` on its next apply, which
  plans an update to fetch it, as `dashboard_password` did.

## 0.12.0 (2026-11-14)

### Changed

- `deevnet_tenant` exports `metrics_endpoint`: the tenant's metrics store, written with Prometheus
  remote write and queried with the Prometheus API using the log tokens it already exports
  (ADR-0023, CHG-0070). Empty at a site with no metrics store. Needs Deevnet API v0.24.0; with an
  older API it reads as empty.

### Documentation

- `deevnet_bucket`: the quota note is corrected. Only a single-part upload larger than the quota
  is refused at once; a multipart upload, how most clients send a large object, is counted after it
  completes, so one object can take a bucket past its quota (found in CHG-0069's verification).

## 0.11.0 (2026-11-14)

Needs API v0.23.0 for `deevnet_bucket` and `deevnet_bucket_key`, at a site that offers buckets.

### Added

- **`deevnet_bucket`**: a bucket in your site's bucket store, S3-compatible object storage at
  `endpoint` (ADR-0026). `quota_gb` changes in place, as do `versioning`, `object_lifecycle` and
  `public_read`. Creating it also creates its `default` key, whose `secret_key` the API returns only
  then, so your state is the only copy you can read. The substrate does not back up a bucket's
  contents: keep your own copy of anything you cannot re-derive. The quota is enforced about a
  minute behind, though an object larger than the quota is refused at once. `public_read` serves
  objects to the site network, never the internet. A bucket that holds anything is destroyed only
  with `force_destroy = true`, applied first.
- **`deevnet_bucket_key`**: a further `read` or `write` key to one bucket. Its `secret_key` is
  likewise returned only on create, and any change replaces the key.

### Upgrading

- Nothing changes for a configuration that does not use the new resources. The expiry block is
  `object_lifecycle`, not `lifecycle`, because Terraform reserves that name. Neither resource can
  be imported: an import could never recover a key's secret.

## 0.10.2 (2026-11-14)

### Fixed

- **A restore takes up a reissued state secret.** After the operator reissues a tenant's state
  secret, its Terraform state still holds the old one. With API 0.22.5 a restore keeps the reissued
  secret and returns it; the provider now plans `state_secret_key` as known after apply on every
  restore, so state takes the new value instead of failing with an inconsistent result.

## 0.10.1 (2026-11-14)

### Fixed

- **A tenant older than its site's log store or dashboards now gets their credentials.** Its state
  never received `log_ingest_token`, `log_read_token` or `dashboard_password`, and a read never
  carries them, so they stayed null and any output built from them failed to plan. The provider now
  plans an update when the site has the service but state has no credential for it; the update is a
  restore, which the API answers with them. Nothing else about the tenant changes.

## 0.10.0 (2026-11-14)

Needs API v0.22.0 for `deevnet_secret` and `deevnet_secret_reader`, at a site that offers tenant
secrets.

### Added

- **`deevnet_secret`**: one of your own runtime secrets, such as a third-party API key, written to
  your OpenBao namespace (ADR-0021). `data_wo` is write-only, so the value never enters your plan or
  state; this needs Terraform 1.11 or later. Raise `data_wo_version` to write a new value. A secret
  OpenBao no longer holds is written again by the next apply, from your configuration.
- **`deevnet_secret_reader`**: the AppRole credential one workload reads its secrets with, for the
  paths you list. Push `role_id` and `secret_id` with your code and run OpenBao Agent with AppRole
  auto-auth against `addr` in `namespace`. It works only from the workload's own address, and every
  change issues a new `secret_id`.

### Upgrading

- Nothing changes for a configuration that does not use the new resources. Using `deevnet_secret`
  needs Terraform 1.11 or later.

## 0.9.0 (2026-11-04)

### Added

- **Windows builds**: `windows_amd64` and `windows_arm64` zips in every release, on GitHub and the
  site's tenant downloads, beside macOS and Linux.
- **`install-provider.ps1` and `tenant-check.ps1`**: the two scripts for Windows PowerShell 5.1 or
  7, with nothing else to install. The provider goes into Terraform's Windows mirror,
  `%APPDATA%\terraform.d\plugins`.
- **`tenant-check.sh` under WSL2**: it reports the networking mode, a checkout on a Windows drive,
  and a name the site's DNS does not resolve.

## 0.8.0 (2026-10-25)

Needs API v0.15.0 for `sites`.

### Added

- **`sites` on `deevnet_certificate`**: the sites to name the certificate at, beside its
  site-less names, such as `["mobile", "home"]` (ADR-0040).

### Changed

- **A certificate's names carry no site first** (API v0.15.0): its common name is
  `<name>.<tenant>.deevnet.net`, then each of `names` the same way, then each label at each of
  `sites`.

### Upgrading

- The next apply after the API moves to v0.15.0 reissues each certificate with the new names,
  for the same key. Whatever serves it needs the new one.

## 0.7.0 (2026-10-11)

Needs API v0.11.0 for `deevnet_certificate`.

### Added

- **`deevnet_certificate`**: a server certificate for one of the tenant's own services, for a name
  in the tenant's own zone, signed by the site's Tenant Service CA (ADR-0037). You make the key,
  with the `tls` provider, and it never goes to the API; this resource takes a signing request.
  It chains to the Deevnet Root CA. Once fewer than sixty days remain, a plan shows the resource
  changing and the apply renews it.
- A documentation site, https://deevnet.github.io/terraform-provider-deevnet/. The resource
  reference is generated from the schemas, and CI fails when it is stale.
- A description on every attribute. The `tenant` argument had none on five resources.
- An example for the provider block and for each resource, under `examples/`.

### Upgrading

- A certificate expires after a year and nothing renews it but an apply. A tenant that holds one
  should plan at least every ten months; the plan shows the renewal when it is due.
- `deevnet_certificate` needs the `hashicorp/tls` provider beside this one, for the key and the
  signing request.

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
