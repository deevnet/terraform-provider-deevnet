terraform {
  required_providers {
    deevnet = {
      source  = "deevnet/deevnet"
      version = "~> 0.6"
    }
  }
}

# Every argument falls back to an environment variable, so the block is
# usually empty and nothing about the site is written into the code.
provider "deevnet" {
  # endpoint       = DEEVNET_API_ENDPOINT
  # token          = DEEVNET_API_TOKEN
  # ca_certificate = DEEVNET_API_CACERT
}
