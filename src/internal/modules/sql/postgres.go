package sql

import (
	"fmt"
	"strings"

	"antelope/internal/modules/setting"
)

// ToPostgresDSN converts a DBConfig into a libpq-style PostgreSQL connection string
// suitable for use with gorm.io/driver/postgres.
//
// Example output:
//
//	host=localhost user=app password=secret dbname=mydb port=5432 sslmode=disable TimeZone=UTC
func ToPostgresDSN(cfg setting.DBConfig) string {
	sslMode := cfg.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=UTC",
		cfg.Host, cfg.Username, cfg.Password, cfg.DB, cfg.Port, sslMode,
	)
}

// RedactDSN masks the password field of a libpq-style DSN so the connection
// string can be safely embedded in error messages and logs. Every other field
// (host, user, dbname, port, sslmode) is preserved for debuggability.
//
// The pgx driver already redacts the password in its own connection errors;
// this helper is for DSN strings we format into errors ourselves (e.g. ping
// failures), where the raw password would otherwise leak into panic output.
func RedactDSN(dsn string) string {
	fields := strings.Fields(dsn)
	for i, f := range fields {
		if strings.HasPrefix(f, "password=") {
			fields[i] = "password=xxxxx"
		}
	}
	return strings.Join(fields, " ")
}
