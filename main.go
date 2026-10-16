// terraform-provider-watchpoint is the Terraform provider for the Watchpoint
// observability platform. See https://api.everwatch-labs.io for API docs.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// version is set at build time via -ldflags. "dev" is the local default.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/everwatch-labs/watchpoint",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}
