package db

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// activeRecordClasses are the SQLSTATEs ActiveRecord's PostgreSQL adapter
// translates to their own exception classes (translate_exception); every
// other server error is ActiveRecord::StatementInvalid.
var activeRecordClasses = map[string]string{
	"22003": "ActiveRecord::RangeError",
	"22001": "ActiveRecord::ValueTooLong",
	"23502": "ActiveRecord::NotNullViolation",
	"23503": "ActiveRecord::InvalidForeignKey",
	"23505": "ActiveRecord::RecordNotUnique",
	"40001": "ActiveRecord::SerializationFailure",
	"40P01": "ActiveRecord::Deadlocked",
	"55P03": "ActiveRecord::LockWaitTimeout",
	"57014": "ActiveRecord::QueryCanceled",
}

// pgClass ports the pg gem's lookup_error_class: the SQLSTATE, else its
// two-character class, else PG::ServerError.
func pgClass(code string) string {
	if c, ok := pgErrorClasses[code]; ok {
		return c
	}
	if len(code) >= 2 {
		if c, ok := pgErrorClasses[code[:2]]; ok {
			return c
		}
	}
	return "PG::ServerError"
}

// RubyError renders a PostgreSQL error the way ActiveRecord reports it:
// the AR class and "PG::Class: ERROR:  message\nDETAIL:  detail\n". ok is
// false for errors that did not come from the server.
func RubyError(err error) (class, message string, ok bool) {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return "", "", false
	}
	arClass, known := activeRecordClasses[pg.Code]
	if !known {
		arClass = "ActiveRecord::StatementInvalid"
	}
	msg := pgClass(pg.Code) + ": " + pg.Severity + ":  " + pg.Message + "\n"
	if pg.Detail != "" {
		msg += "DETAIL:  " + pg.Detail + "\n"
	}
	return arClass, msg, true
}
