// Package specs provides embedded OpenAPI specifications and
// generated types for the mock API server.
package specs

import "embed"

// SpecFS contains the embedded OpenAPI YAML specification files.
//
//go:embed auth.yaml identity.yaml jira.yaml confluence.yaml
var SpecFS embed.FS
