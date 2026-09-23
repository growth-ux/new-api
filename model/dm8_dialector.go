package model

import (
	"strings"

	dameng "github.com/godoes/gorm-dameng"
	"gorm.io/gorm"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

// dm8Dialector wraps the fixed community gorm-dameng dialector to close two
// gaps that were reproduced against the target DM8 build, without forking the
// driver or changing any other dialect:
//
//   - DM8 rejects the JSON base type ("非法的基类名[JSON]"), so JSON and other
//     large-text custom `type:` tags are mapped to CLOB. CLOB round-trips
//     nil/empty/中文/emoji and large values through the existing Scanner/Valuer.
//   - Migrator.GetIndexes reports unique indexes as non-unique (the driver
//     assigns NON_UNIQUE to UniqueValue); dm8Migrator corrects it.
//   - Partial (WHERE) unique indexes are unsupported; dm8Migrator rewrites the
//     single soft-delete case into an equivalent CASE expression index.
//
// Identifier quoting, bind vars, savepoints, Explain and every other type
// mapping are inherited from the embedded driver dialector. The target instance
// runs CASE_SENSITIVE=0, so the driver's double-quoted identifiers already match
// model columns case-insensitively and physical names are intentionally not
// rewritten here.
type dm8Dialector struct {
	dameng.Dialector
}

var (
	_ gorm.Dialector                     = (*dm8Dialector)(nil)
	_ gorm.SavePointerDialectorInterface = (*dm8Dialector)(nil)
)

func newDM8Dialector(dsn string) *dm8Dialector {
	return &dm8Dialector{Dialector: dameng.Dialector{Config: &dameng.Config{
		DriverName: dameng.DriverName,
		DSN:        dsn,
		// The target instance is created with LENGTH_IN_CHAR=1, so the driver reports
		// VARCHAR length in bytes (size*4). Enabling this makes the driver emit
		// VARCHAR(n CHAR) and compare sizes consistently, so re-migration does not
		// re-issue ALTER MODIFY on every sized string column (verified).
		VarcharSizeIsCharLength: true,
	}}}
}

// dm8ClobType maps a custom GORM `type:` tag that DM8 cannot store verbatim to
// its large-character-object equivalent. It returns "" when the driver's own
// mapping should be used unchanged.
func dm8ClobType(field *schema.Field) string {
	switch strings.ToLower(strings.TrimSpace(string(field.DataType))) {
	case "json", "longtext", "mediumtext":
		return "CLOB"
	default:
		return ""
	}
}

func (d *dm8Dialector) DataTypeOf(field *schema.Field) string {
	if clob := dm8ClobType(field); clob != "" {
		return clob
	}
	if field.DataType == schema.Time {
		// DM8 accepts TIMESTAMP WITH TIME ZONE but reports such columns back as
		// DATETIME WITH TIME ZONE. The driver's MigrateColumn treats that as a type
		// change and re-alters the column on every migration; emitting the reported
		// name keeps re-migration idempotent (verified against the target build).
		return "DATETIME WITH TIME ZONE"
	}
	return d.Dialector.DataTypeOf(field)
}

// Explain renders boolean bind values as 1/0. GORM inlines column DEFAULT values
// through Explain, and DM8 rejects `DEFAULT false`/`DEFAULT true` on BIT columns
// ("DEFAULT约束表达式无效"). The driver only converts bool defaults to 1/0 inside
// CreateTable (mutating the cached schema), so a fresh process migrating an
// existing table would otherwise emit an invalid DEFAULT and fail startup.
// Converting here makes the DDL valid; Explain never affects actual bound query
// parameters, which the driver sends to DM8 natively.
func (d *dm8Dialector) Explain(sql string, vars ...any) string {
	hasBool := false
	for _, v := range vars {
		switch v.(type) {
		case bool, *bool:
			hasBool = true
		}
		if hasBool {
			break
		}
	}
	if !hasBool {
		return d.Dialector.Explain(sql, vars...)
	}
	converted := make([]any, len(vars))
	for i, v := range vars {
		switch b := v.(type) {
		case bool:
			converted[i] = dm8BoolLiteral(b)
		case *bool:
			if b == nil {
				converted[i] = v
			} else {
				converted[i] = dm8BoolLiteral(*b)
			}
		default:
			converted[i] = v
		}
	}
	return d.Dialector.Explain(sql, converted...)
}

func dm8BoolLiteral(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Migrator builds the driver's concrete migrator but points GORM's generic type
// resolution (Config.Dialector) back at this wrapper, so CreateTable/AlterColumn
// and the driver's own MigrateColumn emit the CLOB mapping instead of the raw
// driver's "json". Returning the concrete dameng.Migrator (not a wrapper type) is
// required: the driver internally type-asserts db.Migrator().(dameng.Migrator),
// which a wrapper type would panic on. Index-metadata and partial-index fixes are
// applied around migration instead (see dm8_migrator.go).
func (d *dm8Dialector) Migrator(db *gorm.DB) gorm.Migrator {
	return dameng.Migrator{
		Migrator: migrator.Migrator{Config: migrator.Config{
			DB:                          db,
			Dialector:                   d,
			CreateIndexAfterCreateTable: true,
		}},
		Dialector: d.Dialector,
	}
}
