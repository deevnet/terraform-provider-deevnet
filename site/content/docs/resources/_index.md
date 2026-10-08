---
title: "Resources"
weight: 3
---

# Resources

**The provider has seven resources and no data sources.** Each page is generated from the
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
