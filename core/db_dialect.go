package core

// DBDialect identifies the database backend dialect used by the app.
type DBDialect string

const (
	DBDialectSQLite   DBDialect = "sqlite"
	DBDialectPostgres DBDialect = "postgres"
)

// String returns the string representation of the dialect.
func (d DBDialect) String() string {
	return string(d)
}