package db

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// pgClasses maps SQLSTATE codes to the pg gem's exception class and the
// ActiveRecord class that wraps it.
var pgClasses = map[string][2]string{
	"2201W": {"PG::InvalidRowCountInLimitClause", "ActiveRecord::StatementInvalid"},
	"2201X": {"PG::InvalidRowCountInResultOffsetClause", "ActiveRecord::StatementInvalid"},
	"22P02": {"PG::InvalidTextRepresentation", "ActiveRecord::StatementInvalid"},
	"22003": {"PG::NumericValueOutOfRange", "ActiveRecord::RangeError"},
	"22001": {"PG::StringDataRightTruncation", "ActiveRecord::ValueTooLong"},
	"22007": {"PG::InvalidDatetimeFormat", "ActiveRecord::StatementInvalid"},
	"22008": {"PG::DatetimeFieldOverflow", "ActiveRecord::StatementInvalid"},
	"23502": {"PG::NotNullViolation", "ActiveRecord::NotNullViolation"},
	"23503": {"PG::ForeignKeyViolation", "ActiveRecord::InvalidForeignKey"},
	"23505": {"PG::UniqueViolation", "ActiveRecord::RecordNotUnique"},
	"23514": {"PG::CheckViolation", "ActiveRecord::StatementInvalid"},
}

// RubyError renders a PostgreSQL error the way ActiveRecord reports it:
// the AR class and "PG::Class: ERROR:  message\nDETAIL:  detail\n". ok is
// false for errors that did not come from the server.
func RubyError(err error) (class, message string, ok bool) {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) {
		return "", "", false
	}
	names, known := pgClasses[pg.Code]
	if !known {
		names = [2]string{"PG::Error", "ActiveRecord::StatementInvalid"}
	}
	msg := names[0] + ": " + pg.Severity + ":  " + pg.Message + "\n"
	if pg.Detail != "" {
		msg += "DETAIL:  " + pg.Detail + "\n"
	}
	return names[1], msg, true
}
