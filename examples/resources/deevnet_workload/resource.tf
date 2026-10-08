resource "deevnet_workload" "web" {
  tenant    = deevnet_tenant.this.name
  name      = "web"
  cores     = 2
  memory_mb = 2048

  # The public half; the private key never leaves you.
  ssh_keys = [file("~/.ssh/id_ed25519.pub")]
}

output "login" {
  value = "ssh ${deevnet_workload.web.login_user}@${deevnet_workload.web.fqdn}"
}
