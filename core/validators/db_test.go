package validators_test

import (
	"errors"
	"fmt"
	"testing"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core/validators"
	"github.com/pocketbase/pocketbase/tests"
)

type fakeSQLStateErr struct {
	msg   string
	state string
}

func (e fakeSQLStateErr) Error() string {
	return e.msg
}

func (e fakeSQLStateErr) SQLState() string {
	return e.state
}

func TestUniqueId(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	scenarios := []struct {
		id          string
		tableName   string
		expectError bool
	}{
		{"", "", false},
		{"test", "", true},
		{"wsmn24bux7wo113", "_collections", true},
		{"test_unique_id", "unknown_table", true},
		{"test_unique_id", "_collections", false},
	}

	for i, s := range scenarios {
		t.Run(fmt.Sprintf("%d_%s_%s", i, s.id, s.tableName), func(t *testing.T) {
			err := validators.UniqueId(app.DB(), s.tableName)(s.id)

			hasErr := err != nil
			if hasErr != s.expectError {
				t.Fatalf("Expected hasErr to be %v, got %v (%v)", s.expectError, hasErr, err)
			}
		})
	}
}

func TestNormalizeUniqueIndexError(t *testing.T) {
	t.Parallel()

	scenarios := []struct {
		name         string
		err          error
		table        string
		names        []string
		expectedKeys []string
	}{
		{
			"nil error (no changes)",
			nil,
			"test",
			[]string{"a", "b"},
			nil,
		},
		{
			"non-unique index error (no changes)",
			errors.New("abc"),
			"test",
			[]string{"a", "b"},
			nil,
		},
		{
			"validation error (no changes)",
			validation.Errors{"c": errors.New("abc")},
			"test",
			[]string{"a", "b"},
			[]string{"c"},
		},
		{
			"unique index error but mismatched table name",
			errors.New("UNIQUE constraint failed for fields test.a,test.b"),
			"example",
			[]string{"a", "b"},
			nil,
		},
		{
			"unique index error with table name suffix matching the specified one",
			errors.New("UNIQUE constraint failed for fields test_suffix.a,test_suffix.b"),
			"suffix",
			[]string{"a", "b", "c"},
			nil,
		},
		{
			"unique index error but mismatched fields",
			errors.New("UNIQUE constraint failed for fields test.a,test.b"),
			"test",
			[]string{"c", "d"},
			nil,
		},
		{
			"unique index error with matching table name and fields",
			errors.New("UNIQUE constraint failed for fields test.a,test.b"),
			"test",
			[]string{"a", "b", "c"},
			[]string{"a", "b"},
		},
		{
			"unique index error with matching table name and field starting with the name of another non-unique field",
			errors.New("UNIQUE constraint failed for fields test.a_2,test.c"),
			"test",
			[]string{"a", "a_2", "c"},
			[]string{"a_2", "c"},
		},
		{
			"postgres duplicate key with matching single field",
			errors.New(`duplicate key value violates unique constraint "users_email_key" (SQLSTATE 23505) DETAIL: Key (email)=(a@example.com) already exists.`),
			"users",
			[]string{"email", "name"},
			[]string{"email"},
		},
		{
			"postgres duplicate key with matching multi fields",
			errors.New(`duplicate key value violates unique constraint "idx" DETAIL: Key (collectionRef, recordRef)=(users, abc) already exists.`),
			"ignored",
			[]string{"collectionRef", "recordRef", "name"},
			[]string{"collectionRef", "recordRef"},
		},
		{
			"postgres SQLSTATE 23505 with detail",
			fakeSQLStateErr{
				msg:   `db error: Key (tokenKey)=(abc) already exists.`,
				state: "23505",
			},
			"ignored",
			[]string{"tokenKey", "email"},
			[]string{"tokenKey"},
		},
		{
			"wrapped postgres SQLSTATE 23505 with detail",
			fmt.Errorf("wrapped: %w", fakeSQLStateErr{
				msg:   `db error: Key (tokenKey)=(abc) already exists.`,
				state: "23505",
			}),
			"ignored",
			[]string{"tokenKey", "email"},
			[]string{"tokenKey"},
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			result := validators.NormalizeUniqueIndexError(s.err, s.table, s.names)

			if len(s.expectedKeys) == 0 {
				if result != s.err {
					t.Fatalf("Expected no error change, got %v", result)
				}
				return
			}

			tests.TestValidationErrors(t, result, s.expectedKeys)
		})
	}
}
