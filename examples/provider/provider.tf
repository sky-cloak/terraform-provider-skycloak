terraform {
  required_providers {
    skycloak = {
      source  = "sky-cloak/skycloak"
      version = "~> 0.1"
    }
  }
}

provider "skycloak" {
  # api_key is read from the SKYCLOAK_API_KEY environment variable (recommended).
  # endpoint defaults to https://api.skycloak.io
  # api_version defaults to the current API version.
  # platform_api_key is read from SKYCLOAK_PLATFORM_API_KEY, and
  # platform_endpoint defaults to https://api.skycloak.io, for Platform resources.
}
