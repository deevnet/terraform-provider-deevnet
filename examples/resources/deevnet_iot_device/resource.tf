# Registering a device grants it nothing. It is identity, not authorization.
resource "deevnet_iot_device" "stand" {
  tenant      = deevnet_tenant.this.name
  name        = "stand-1"
  trust_class = "iot"

  # Needed only when the device asks for a fixed address. Never authorization:
  # a MAC is trivially spoofed on a shared segment.
  mac = "aa:bb:cc:dd:ee:ff"
}
