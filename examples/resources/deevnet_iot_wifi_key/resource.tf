# One key per tenant per trust class: every device the tenant flashes with it
# shares it, so revoking it stops all of them.
resource "deevnet_iot_wifi_key" "devices" {
  tenant      = deevnet_tenant.this.name
  name        = "devices"
  trust_class = "iot"
}

# Flash devices with these two. Never hardcode the SSID: the same trust class
# is a different SSID at another site.
output "device_wifi" {
  value = {
    ssid = deevnet_iot_wifi_key.devices.ssid
    psk  = deevnet_iot_wifi_key.devices.psk
  }
  sensitive = true
}
