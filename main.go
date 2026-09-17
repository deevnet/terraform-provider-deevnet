// terraform-provider-deevnet serves the deevnet/deevnet provider.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/deevnet/terraform-provider-deevnet/internal/provider"
)

// Set at link time by the release build.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		// The source address tenants write, public registry form from day one
		// (ADR-0012 §7).
		Address: "registry.terraform.io/deevnet/deevnet",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
