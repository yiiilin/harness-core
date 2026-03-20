package postgres

import _ "embed"

//go:embed schema.sql
var schemaSQL string

// SchemaSQL returns the canonical bootstrap SQL for the durable Postgres runtime.
func SchemaSQL() string {
	return schemaSQL
}
