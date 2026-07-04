# Forward security logs to a syslog collector over TLS.
resource "skycloak_siem_destination" "syslog" {
  name = "corporate-siem"
  type = "syslog"

  source = {
    type = "security_logs"
  }

  syslog = {
    host     = "siem.example.com"
    port     = 6514
    protocol = "tls"
    format   = "rfc5424"
  }
}

# Forward Keycloak login events to an HTTP collector (e.g. Splunk HEC).
resource "skycloak_siem_destination" "splunk" {
  name = "splunk-hec"
  type = "http"

  source = {
    type                 = "keycloak_events"
    keycloak_event_types = ["LOGIN", "LOGIN_ERROR", "LOGOUT"]
  }

  http = {
    url          = "https://hec.example.com/services/collector"
    auth_type    = "bearer"
    bearer_token = var.splunk_hec_token # write-only; never read back
  }

  batch = {
    max_events           = 500
    max_interval_seconds = 30
  }
}
