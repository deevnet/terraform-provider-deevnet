# The credential the lightd workload reads its secrets with. It works only
# from that workload's own address.
resource "deevnet_secret_reader" "lightd" {
  tenant   = deevnet_tenant.this.name
  name     = "lightd"
  workload = deevnet_workload.lightd.name
  paths    = [deevnet_secret.weather.path]
}

# Push these with your code, and point OpenBao Agent's AppRole auto-auth at
# them: role_id and secret_id as the method's files, addr as vault.address,
# and namespace as the namespace. Any change to the reader issues a new
# secret_id, so push again after it.
output "lightd_bao" {
  value = {
    addr      = deevnet_secret_reader.lightd.addr
    namespace = deevnet_secret_reader.lightd.namespace
    role_id   = deevnet_secret_reader.lightd.role_id
  }
}

output "lightd_bao_secret_id" {
  value     = deevnet_secret_reader.lightd.secret_id
  sensitive = true
}
