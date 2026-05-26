resource "skycloak_cluster_security" "production" {
  cluster_id = skycloak_cluster.production.id

  # Lock the admin console down to the office network.
  ip_access_control = [
    {
      path          = "/admin"
      description   = "Office network only"
      allowed_cidrs = ["203.0.113.0/24"]
    },
  ]

  rate_limiting = {
    enabled    = true
    global_rpm = 60000
    per_ip_rpm = 600
  }

  waf = {
    enabled        = true
    mode           = "block"
    preset         = "owasp_top_10"
    paranoia_level = 1
  }

  geo_blocking = {
    enabled   = true
    mode      = "blocklist"
    countries = ["KP", "RU"]
  }

  bot_management = {
    enabled        = true
    mode           = "block"
    challenge_mode = "javascript"
  }
}
