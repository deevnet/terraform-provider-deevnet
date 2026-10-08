resource "deevnet_iot_broker_account" "stand" {
  tenant = deevnet_tenant.this.name
  name   = "stand-1"
  device = deevnet_iot_device.stand.name

  # Relative to the tenant. The API writes the "tdemo/" prefix itself, which is
  # what confines a tenant to its own topics.
  publish   = ["lightstand/+/telemetry"]
  subscribe = ["lightstand/+/command"]
}

output "broker" {
  value = {
    username = deevnet_iot_broker_account.stand.username
    password = deevnet_iot_broker_account.stand.password
    # What the broker actually enforces, prefix and all.
    publish = deevnet_iot_broker_account.stand.granted_publish
  }
  sensitive = true
}
