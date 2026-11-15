# Alerts over the tenant's own metrics, to the ntfy topic "<tenant>-backend".
resource "deevnet_alert_rule_group" "backend" {
  tenant = deevnet_tenant.this.name
  name   = "backend"
  type   = "metrics"
  topic  = "backend"

  rule {
    alert       = "QueueBacklog"
    expr        = "backend_queue_depth > 100"
    for         = "5m"
    labels      = { severity = "page" }
    annotations = { summary = "Queue is {{ $value }}" }
  }
}

# Alerts over the devices' logs from MQTT. A logs rule is LogsQL ending in a
# stats pipe; it fires for each row the query returns.
resource "deevnet_alert_rule_group" "doors" {
  tenant    = deevnet_tenant.this.name
  name      = "doors"
  type      = "logs"
  partition = "devices"
  topic     = "devices"

  rule {
    alert       = "DoorOpen"
    expr        = "\"door open\" | stats by (device) count() as hits | filter hits:>0"
    annotations = { summary = "{{ $labels.device }} opened {{ $value }} times" }
  }
}

# On the phone: the ntfy app, server notify_endpoint, logged in with
# notify_username and notify_password, subscribed to each group's ntfy_topic.
output "alert_topics" {
  value = [deevnet_alert_rule_group.backend.ntfy_topic, deevnet_alert_rule_group.doors.ntfy_topic]
}
