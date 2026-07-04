resource "skycloak_captcha_domain" "login" {
  cluster_id = skycloak_cluster.production.id
  hostname   = "login.example.com"
}
