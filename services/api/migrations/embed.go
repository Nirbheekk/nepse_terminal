// Package migrations embeds the SQL migration set so the API can migrate itself
// without shipping a separate tool or reading from disk.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
