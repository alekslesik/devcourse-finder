package main

import (
	"encoding/json"
	"errors"
	"log/slog"

	"devcourse-finder/catalog"
	"github.com/jackc/pgx/v5/pgconn"
)

// Keep raw causes for classification, never for log output: database errors and
// decoder/file errors can contain credentials, URLs, paths or catalog values.
type commandError struct {
	stage, code, message string
	cause                error
}

func (e *commandError) Error() string { return e.message }
func (e *commandError) Unwrap() error { return e.cause }
func commandFailure(stage, code, message string, cause error) error {
	return &commandError{stage: stage, code: code, message: message, cause: cause}
}
func commandName(args []string) string {
	if len(args) == 0 {
		return "serve"
	}
	if args[0] == "catalog" && len(args) > 1 {
		switch args[1] {
		case "import":
			return "catalog_import"
		case "validate":
			return "catalog_validate"
		}
	}
	switch args[0] {
	case "migrate", "report", "cleanup":
		return args[0]
	}
	return "unknown"
}
func commandErrorAttrs(err error) []slog.Attr {
	code, stage, message := "command_failed", "execute", "Command failed"
	var failure *commandError
	if errors.As(err, &failure) {
		code, stage, message = failure.code, failure.stage, failure.message
	}
	attrs := []slog.Attr{slog.String("code", code), slog.String("stage", stage), slog.String("error", message)}
	var validation *catalog.ValidationError
	if errors.As(err, &validation) {
		attrs = append(attrs, slog.Int("course_index", validation.Index), slog.String("validation_rule", validation.Rule))
	}
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		attrs = append(attrs, slog.Int64("json_offset", syntax.Offset))
	}
	var field *json.UnmarshalTypeError
	if errors.As(err, &field) {
		attrs = append(attrs, slog.Int64("json_offset", field.Offset))
	}
	var connection *pgconn.ConnectError
	if errors.As(err, &connection) {
		attrs = append(attrs, slog.String("failure_kind", "database_connection"))
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && safeSQLState(pg.Code) {
		attrs = append(attrs, slog.String("sql_state", pg.Code))
		// These schema-owned names help identify the failure without logging Detail,
		// Message, SQL or data-derived constraint names.
		switch pg.ConstraintName {
		case "courses_pkey", "courses_slug_key", "offers_pkey", "offers_course_id_fkey", "offers_price_check", "imports_pkey", "settings_pkey":
			attrs = append(attrs, slog.String("constraint", pg.ConstraintName))
		}
	}
	return attrs
}
func safeSQLState(value string) bool {
	if len(value) != 5 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}
