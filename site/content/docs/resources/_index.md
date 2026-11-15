---
title: "Resources"
weight: 3
---

# Resources

**The provider has thirteen resources and no data sources.** Each page is generated from the
resource's schema.

| Resource | What it is |
|---|---|
| [`deevnet_tenant`](/docs/resources/tenant/) | the tenant: index, numbering, DNS zone and key, state-store credential, API token |
| [`deevnet_workload`](/docs/resources/workload/) | a VM in the tenant's network; the API derives its VMID, MAC and address |
| [`deevnet_dns_record`](/docs/resources/dns_record/) | a name in the tenant's zone, with its PTR |
| [`deevnet_iot_wifi_key`](/docs/resources/iot_wifi_key/) | a Wi-Fi key for the tenant's devices, bound to its trust class's VLAN |
| [`deevnet_iot_device`](/docs/resources/iot_device/) | an entry in the tenant's device registry: an identity, carrying no credential |
| [`deevnet_iot_address`](/docs/resources/iot_address/) | a fixed address on the device network for a registered device, and its name in the tenant's zone |
| [`deevnet_iot_broker_account`](/docs/resources/iot_broker_account/) | an MQTT account on the platform broker, for a device or a workload |
| [`deevnet_certificate`](/docs/resources/certificate/) | a server certificate for one of the tenant's own services, for a name in its zone; the tenant keeps the key |
| [`deevnet_secret`](/docs/resources/secret/) | a runtime secret in the tenant's own OpenBao namespace, from a write-only value |
| [`deevnet_secret_reader`](/docs/resources/secret_reader/) | a workload's login to that namespace, reading only the paths named |
| [`deevnet_bucket`](/docs/resources/bucket/) | an S3 bucket in the site's bucket store, with a quota |
| [`deevnet_bucket_key`](/docs/resources/bucket_key/) | a key to one bucket, read-only or read-write |
| [`deevnet_alert_rule_group`](/docs/resources/alert_rule_group/) | alert rules over the tenant's metrics or logs, delivered to its ntfy topic on site |
