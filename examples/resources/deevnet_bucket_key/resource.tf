# A read-only key for a workload that only serves the bucket's objects.
resource "deevnet_bucket_key" "media_reader" {
  tenant = deevnet_tenant.this.name
  bucket = deevnet_bucket.media.name
  name   = "reader"
  access = "read"
}

output "media_reader" {
  value = {
    access_key = deevnet_bucket_key.media_reader.access_key
    secret_key = deevnet_bucket_key.media_reader.secret_key
  }
  sensitive = true
}
