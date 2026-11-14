resource "deevnet_bucket" "media" {
  tenant   = deevnet_tenant.this.name
  name     = "media"
  quota_gb = 10

  versioning = true

  # Not "lifecycle": Terraform reserves that name for its own meta-argument.
  object_lifecycle {
    noncurrent_expire_days = 30
  }

  # Set this and apply before a destroy that should empty the bucket.
  force_destroy = false
}

# The store does not back up the bucket. Keep your own copy of anything you
# cannot re-derive.
output "media_bucket" {
  value = {
    endpoint   = deevnet_bucket.media.endpoint
    bucket     = deevnet_bucket.media.bucket
    access_key = deevnet_bucket.media.access_key
    secret_key = deevnet_bucket.media.secret_key
  }
  sensitive = true
}
