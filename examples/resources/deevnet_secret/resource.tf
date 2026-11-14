# data_wo is write-only: it never enters your plan or state, and needs
# Terraform 1.11 or later. Keep the value encrypted in your own repository and
# pass it in, here as a sensitive variable.
variable "weather_api_key" {
  type      = string
  sensitive = true
}

resource "deevnet_secret" "weather" {
  tenant = deevnet_tenant.this.name
  path   = "weather/api"

  data_wo = {
    api_key = var.weather_api_key
  }

  # Terraform cannot see a write-only value change. Raise this with every new
  # value, and the apply writes it.
  data_wo_version = 1
}
