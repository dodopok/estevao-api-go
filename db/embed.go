// Package dbfiles embeds the database schema snapshot and the migrations,
// so a binary always carries exactly the schema it was built against.
package dbfiles

import "embed"

// Schema is db/schema.sql: the full schema after every migration, and the
// versions applied, loaded into a new empty database by `estevao db prepare`.
//
//go:embed schema.sql
var Schema string

// Migrations holds db/migrations/*.sql.
//
//go:embed migrations
var Migrations embed.FS
