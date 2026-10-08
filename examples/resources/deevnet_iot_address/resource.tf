# A fixed address on the device network, so the device is found at the same
# place every time it joins. The API picks it and this state remembers it; the
# device is also published as stand-1.<tenant zone>.
resource "deevnet_iot_address" "stand" {
  tenant = deevnet_tenant.this.name
  device = deevnet_iot_device.stand.name
}

output "stand_address" {
  value = deevnet_iot_address.stand.address
}
