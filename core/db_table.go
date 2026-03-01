package core

import (
	"database/sql"
	"fmt"

	"github.com/pocketbase/dbx"
)

// TableColumns returns all column names of a single table by its name.
func (app *BaseApp) TableColumns(tableName string) ([]string, error) {
	columns := []string{}

	if app.DBDialect() == DBDialectPostgres {
		err := app.ConcurrentDB().NewQuery(`
			SELECT c.column_name
			FROM information_schema.columns c
			WHERE c.table_schema = current_schema()
			  AND LOWER(c.table_name) = LOWER({:tableName})
			ORDER BY c.ordinal_position
		`).Bind(dbx.Params{"tableName": tableName}).Column(&columns)

		return columns, err
	}

	err := app.ConcurrentDB().NewQuery("SELECT name FROM PRAGMA_TABLE_INFO({:tableName})").
		Bind(dbx.Params{"tableName": tableName}).
		Column(&columns)

	return columns, err
}

type TableInfoRow struct {
	// the `db:"pk"` tag has special semantic so we cannot rename
	// the original field without specifying a custom mapper
	PK int

	Index        int            `db:"cid"`
	Name         string         `db:"name"`
	Type         string         `db:"type"`
	NotNull      bool           `db:"notnull"`
	DefaultValue sql.NullString `db:"dflt_value"`
}

// TableInfo returns the "table_info" pragma result for the specified table.
func (app *BaseApp) TableInfo(tableName string) ([]*TableInfoRow, error) {
	info := []*TableInfoRow{}

	if app.DBDialect() == DBDialectPostgres {
		err := app.ConcurrentDB().NewQuery(`
			SELECT
				CASE WHEN tc.constraint_type = 'PRIMARY KEY' THEN 1 ELSE 0 END AS pk,
				(c.ordinal_position - 1) AS cid,
				c.column_name AS name,
				UPPER(c.udt_name) AS type,
				CASE WHEN c.is_nullable = 'NO' THEN 1 ELSE 0 END AS notnull,
				c.column_default AS dflt_value
			FROM information_schema.columns c
			LEFT JOIN information_schema.key_column_usage kcu
				ON kcu.table_schema = c.table_schema
				AND kcu.table_name = c.table_name
				AND kcu.column_name = c.column_name
			LEFT JOIN information_schema.table_constraints tc
				ON tc.table_schema = kcu.table_schema
				AND tc.table_name = kcu.table_name
				AND tc.constraint_name = kcu.constraint_name
				AND tc.constraint_type = 'PRIMARY KEY'
			WHERE c.table_schema = current_schema()
			  AND LOWER(c.table_name) = LOWER({:tableName})
			ORDER BY c.ordinal_position
		`).Bind(dbx.Params{"tableName": tableName}).All(&info)
		if err != nil {
			return nil, err
		}

		if len(info) == 0 {
			return nil, fmt.Errorf("empty table info probably due to invalid or missing table %s", tableName)
		}

		return info, nil
	}

	err := app.ConcurrentDB().NewQuery("SELECT * FROM PRAGMA_TABLE_INFO({:tableName})").
		Bind(dbx.Params{"tableName": tableName}).
		All(&info)
	if err != nil {
		return nil, err
	}

	// mattn/go-sqlite3 doesn't throw an error on invalid or missing table
	// so we additionally have to check whether the loaded info result is nonempty
	if len(info) == 0 {
		return nil, fmt.Errorf("empty table info probably due to invalid or missing table %s", tableName)
	}

	return info, nil
}

// TableIndexes returns a name grouped map with all non empty index of the specified table.
//
// Note: This method doesn't return an error on nonexisting table.
func (app *BaseApp) TableIndexes(tableName string) (map[string]string, error) {
	indexes := []struct {
		Name string
		Sql  string
	}{}

	if app.DBDialect() == DBDialectPostgres {
		err := app.ConcurrentDB().NewQuery(`
			SELECT i.indexname AS name, i.indexdef AS sql
			FROM pg_indexes i
			WHERE i.schemaname = current_schema()
			  AND LOWER(i.tablename) = LOWER({:tableName})
		`).Bind(dbx.Params{"tableName": tableName}).All(&indexes)
		if err != nil {
			return nil, err
		}

		result := make(map[string]string, len(indexes))
		for _, idx := range indexes {
			result[idx.Name] = idx.Sql
		}

		return result, nil
	}

	err := app.ConcurrentDB().Select("name", "sql").
		From("sqlite_master").
		AndWhere(dbx.NewExp("sql is not null")).
		AndWhere(dbx.HashExp{
			"type":     "index",
			"tbl_name": tableName,
		}).
		All(&indexes)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(indexes))

	for _, idx := range indexes {
		result[idx.Name] = idx.Sql
	}

	return result, nil
}

// DeleteTable drops the specified table.
//
// This method is a no-op if a table with the provided name doesn't exist.
//
// NB! Be aware that this method is vulnerable to SQL injection and the
// "dangerousTableName" argument must come only from trusted input!
func (app *BaseApp) DeleteTable(dangerousTableName string) error {
	_, err := app.NonconcurrentDB().NewQuery(fmt.Sprintf(
		"DROP TABLE IF EXISTS {{%s}}",
		dangerousTableName,
	)).Execute()

	return err
}

// HasTable checks if a table (or view) with the provided name exists (case insensitive).
// in the data.db.
func (app *BaseApp) HasTable(tableName string) bool {
	return app.hasTable(app.ConcurrentDB(), tableName)
}

// AuxHasTable checks if a table (or view) with the provided name exists (case insensitive)
// in the auixiliary.db.
func (app *BaseApp) AuxHasTable(tableName string) bool {
	return app.hasTable(app.AuxConcurrentDB(), tableName)
}

func (app *BaseApp) hasTable(db dbx.Builder, tableName string) bool {
	var exists int

	if app.DBDialect() == DBDialectPostgres {
		err := db.NewQuery(`
			SELECT 1
			FROM pg_catalog.pg_class c
			JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = current_schema()
			  AND c.relkind IN ('r', 'v', 'm', 'p', 'f')
			  AND LOWER(c.relname) = LOWER({:tableName})
			LIMIT 1
		`).Bind(dbx.Params{"tableName": tableName}).Row(&exists)

		return err == nil && exists > 0
	}

	err := db.Select("(1)").
		From("sqlite_schema").
		AndWhere(dbx.HashExp{"type": []any{"table", "view"}}).
		AndWhere(dbx.NewExp("LOWER([[name]])=LOWER({:tableName})", dbx.Params{"tableName": tableName})).
		Limit(1).
		Row(&exists)

	return err == nil && exists > 0
}

// Vacuum executes VACUUM on the data.db in order to reclaim unused data db disk space.
func (app *BaseApp) Vacuum() error {
	return app.vacuum(app.NonconcurrentDB())
}

// AuxVacuum executes VACUUM on the auxiliary.db in order to reclaim unused auxiliary db disk space.
func (app *BaseApp) AuxVacuum() error {
	return app.vacuum(app.AuxNonconcurrentDB())
}

func (app *BaseApp) vacuum(db dbx.Builder) error {
	_, err := db.NewQuery("VACUUM").Execute()

	return err
}
