// Package main is the entry point for the Atlassian OpenTofu/Terraform provider.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version = "dev"

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.opentofu.org/asymmetric-effort/atlassian",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}
