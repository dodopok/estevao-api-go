package migrate

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Dump renders db/schema.sql from the database at url: pg_dump's schema,
// then the applied versions. It needs the pg_dump client of the server's
// major version (a development tool; the image does not carry it).
func Dump(ctx context.Context, url string) (string, error) {
	cmd := exec.CommandContext(ctx, "pg_dump", "--schema-only", "--no-owner", "--no-privileges", "--no-comments", url)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pg_dump: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return "", err
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return "", err
	}
	versions, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("-- Schema of the Estêvão database after every migration, written by\n")
	b.WriteString("-- `estevao db dump`. Never edit by hand: add a migration (`estevao db new`),\n")
	b.WriteString("-- apply it to a local database and dump again.\n\n")
	for _, line := range strings.SplitAfter(out.String(), "\n") {
		if strings.HasPrefix(line, "-- Dumped") || strings.HasPrefix(line, `\restrict`) || strings.HasPrefix(line, `\unrestrict`) {
			continue
		}
		b.WriteString(line)
	}
	b.WriteString("\nSET search_path = public;\n")
	b.WriteString("INSERT INTO schema_migrations (version) VALUES\n")
	for i, v := range versions {
		sep := ",\n"
		if i == len(versions)-1 {
			sep = "\n"
		}
		b.WriteString("('" + v + "')" + sep)
	}
	b.WriteString(";\n")
	return b.String(), nil
}
