package validators

import (
	"database/sql"
	"errors"
	"regexp"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/pocketbase/dbx"
)

var uniqueViolationSQLStates = map[string]struct{}{
	"23505": {},
}

// UniqueId checks whether a field string id already exists in the specified table.
//
// Example:
//
//	validation.Field(&form.RelId, validation.By(validators.UniqueId(form.app.DB(), "tbl_example"))
func UniqueId(db dbx.Builder, tableName string) validation.RuleFunc {
	return func(value any) error {
		v, _ := value.(string)
		if v == "" {
			return nil // nothing to check
		}

		var foundId string

		err := db.
			Select("id").
			From(tableName).
			Where(dbx.HashExp{"id": v}).
			Limit(1).
			Row(&foundId)

		if (err != nil && !errors.Is(err, sql.ErrNoRows)) || foundId != "" {
			return validation.NewError("validation_invalid_or_existing_id", "The model id is invalid or already exists.")
		}

		return nil
	}
}

// NormalizeUniqueIndexError attempts to convert a
// "unique constraint failed" error into a validation.Errors.
//
// The provided err is returned as it is without changes if:
// - err is nil
// - err is already validation.Errors
// - err is not "unique constraint failed" error
func NormalizeUniqueIndexError(err error, tableOrAlias string, fieldNames []string) error {
	if err == nil {
		return err
	}

	if _, ok := err.(validation.Errors); ok {
		return err
	}

	msg := strings.ToLower(err.Error())

	if !isUniqueViolationError(err, msg) {
		return err
	}

	normalizedErrs := validation.Errors{}

	if strings.Contains(msg, "unique constraint failed") {
		// note: extra space to unify multi-columns lookup
		sqliteMsg := strings.ReplaceAll(strings.TrimSpace(msg), ",", " ") + " "

		for _, name := range fieldNames {
			// note: extra spaces to exclude table name with suffix matching the current one
			//       OR other fields starting with the current field name
			if strings.Contains(sqliteMsg, strings.ToLower(" "+tableOrAlias+"."+name+" ")) {
				normalizedErrs[name] = validation.NewError("validation_not_unique", "Value must be unique")
			}
		}
	}

	for _, name := range extractPostgresUniqueFieldNames(err.Error()) {
		for _, allowed := range fieldNames {
			if strings.EqualFold(name, allowed) {
				normalizedErrs[allowed] = validation.NewError("validation_not_unique", "Value must be unique")
				break
			}
		}
	}

	if len(normalizedErrs) > 0 {
		return normalizedErrs
	}

	return err
}

func isUniqueViolationError(err error, msg string) bool {
	if strings.Contains(msg, "unique constraint failed") ||
		strings.Contains(msg, "duplicate key value violates unique constraint") {
		return true
	}

	type sqlStateCarrier interface {
		SQLState() string
	}

	var stateErr sqlStateCarrier
	if errors.As(err, &stateErr) {
		code := strings.ToUpper(strings.TrimSpace(stateErr.SQLState()))
		_, ok := uniqueViolationSQLStates[code]
		return ok
	}

	return false
}

var postgresUniqueFieldsRegex = regexp.MustCompile(`(?i)key\s*\(([^\)]+)\)\s*=`)

func extractPostgresUniqueFieldNames(message string) []string {
	matches := postgresUniqueFieldsRegex.FindStringSubmatch(message)
	if len(matches) != 2 {
		return nil
	}

	parts := strings.Split(matches[1], ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.Trim(strings.TrimSpace(part), `"'[]`)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}
