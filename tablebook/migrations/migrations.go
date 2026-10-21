// Package migrations embeds the SQL schema files.
package migrations

import _ "embed"

// Init creates the reservations table.
//
//go:embed 001_init.sql
var Init string
