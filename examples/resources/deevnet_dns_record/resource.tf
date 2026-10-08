# A second name for a workload, with its PTR.
resource "deevnet_dns_record" "api" {
  tenant  = deevnet_tenant.this.name
  name    = "api"
  address = deevnet_workload.web.address
}
