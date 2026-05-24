// terraform-provider-skycloak is the official Terraform provider for Skycloak
// (managed Keycloak). It manages clusters, realms, applications, identity
// providers, branding and security configuration via the Skycloak public API.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	skycloakprovider "github.com/sky-cloak/terraform-provider-skycloak/internal/provider"
)

// version is overridden at build/release time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/sky-cloak/skycloak",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), skycloakprovider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}
