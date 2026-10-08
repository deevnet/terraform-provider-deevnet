resource "deevnet_tenant" "this" {
  name = "tdemo"
}

output "subnet" {
  value = deevnet_tenant.this.subnet
}

# What the API issued this tenant. This state is the authoritative copy.
output "state_credentials" {
  value = {
    endpoint   = deevnet_tenant.this.state_endpoint
    bucket     = deevnet_tenant.this.state_bucket
    key_prefix = deevnet_tenant.this.state_key_prefix
    access_key = deevnet_tenant.this.state_access_key
    secret_key = deevnet_tenant.this.state_secret_key
  }
  sensitive = true
}
