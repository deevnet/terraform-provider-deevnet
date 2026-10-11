# You make the key, and it stays in your state and on your workload.
# Needs the hashicorp/tls provider.
resource "tls_private_key" "console" {
  algorithm   = "ECDSA"
  ecdsa_curve = "P256"
}

# The request is read for its key alone, so its subject is only a label for
# your own reading.
resource "tls_cert_request" "console" {
  private_key_pem = tls_private_key.console.private_key_pem

  subject {
    common_name = "console"
  }
}

# A certificate for console.<tenant zone>, and for www.<tenant zone> beside it.
resource "deevnet_certificate" "console" {
  tenant  = deevnet_tenant.this.name
  name    = "console"
  names   = ["www"]
  csr_pem = tls_cert_request.console.cert_request_pem
}

# What a server wants: the key, and the certificate with its chain.
output "console_tls_key" {
  value     = tls_private_key.console.private_key_pem
  sensitive = true
}

output "console_tls_fullchain" {
  value = deevnet_certificate.console.fullchain_pem
}
