// Package migrations embeds the SQL migration files for use with golang-migrate.
package migrations

import "embed"

// FS holds all SQL migration files.
//
//go:embed *.sql
var FS embed.FS
