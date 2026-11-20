// Package api embeds the OpenAPI spec, the source of truth for the server handlers and the app's client.
package api

import _ "embed"

// Spec is the raw api/openapi.yaml, served at /openapi.yaml.
//
//go:embed openapi.yaml
var Spec []byte
