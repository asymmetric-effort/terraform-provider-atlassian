// Package main is the entry point for the mock Atlassian API server.
package main

import (
	"flag"
	"log"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	log.Fatal(mock.Run(*addr))
}
