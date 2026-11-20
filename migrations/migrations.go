// Package migrations embeds the SQL migrations so the binary can apply them with goose.
package migrations

import "embed"

// FS holds the goose migration files.
//
//go:embed *.sql
var FS embed.FS
