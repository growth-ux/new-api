package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// The community driver's migrator cannot be wrapped in a distinct type: several of
// its methods type-assert db.Migrator().(dameng.Migrator) and would panic on a
// wrapper. The two metadata gaps that matter for new-api are therefore handled
// around migration with standalone, idempotent steps instead of method overrides.

// dm8QuoteIdent double-quotes a model-controlled identifier for DM8. Identifiers
// originate from struct tags and catalog names, never from request input; embedded
// quotes are escaped by doubling.
func dm8QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// dm8PrefillUniqueIndex mirrors the prefill_groups partial unique index declared
// in the model as `uniqueIndex:uk_prefill_name,where:deleted_at IS NULL`.
const (
	dm8PrefillTable        = "prefill_groups"
	dm8PrefillUniqueIndex  = "uk_prefill_name"
	dm8PrefillUniqueColumn = "name"
	dm8PrefillUniqueCond   = "deleted_at IS NULL"
)

// ensureDM8PrefillUniqueIndex replaces the plain unique index GORM creates for
// prefill_groups.name with an equivalent CASE-expression unique index. GORM's
// generic CreateIndex ignores schema.Index.Where, so on DM8 the partial condition
// would otherwise be dropped and soft-deleted names could never be reused. The
// CASE form was verified against the target DM8 build to enforce uniqueness only
// among rows matching the condition while allowing any number of NULL (deleted)
// rows, matching PostgreSQL partial-index semantics.
//
// It is idempotent: once the index is function-based it is left untouched, so a
// second migration issues no DDL for it.
func ensureDM8PrefillUniqueIndex(db *gorm.DB) error {
	if !db.Migrator().HasTable(&PrefillGroup{}) {
		return nil
	}
	functionBased, err := dm8IndexIsFunctionBased(db, dm8PrefillUniqueIndex)
	if err != nil {
		return err
	}
	if functionBased {
		return nil
	}
	if db.Migrator().HasIndex(&PrefillGroup{}, dm8PrefillUniqueIndex) {
		if err := db.Exec("DROP INDEX " + dm8QuoteIdent(dm8PrefillUniqueIndex)).Error; err != nil {
			return fmt.Errorf("dm8 drop plain prefill unique index: %w", err)
		}
	}
	createSQL := fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (CASE WHEN %s THEN %s ELSE NULL END)",
		dm8QuoteIdent(dm8PrefillUniqueIndex), dm8QuoteIdent(dm8PrefillTable),
		dm8PrefillUniqueCond, dm8QuoteIdent(dm8PrefillUniqueColumn))
	if err := db.Exec(createSQL).Error; err != nil {
		return fmt.Errorf("dm8 create prefill expression unique index: %w", err)
	}
	return nil
}

// dm8IndexIsFunctionBased reports whether an index is expression-based. DM8 marks
// the columns of a function-based index with a negative COLUMN_POSITION in
// USER_IND_COLUMNS (verified on the target build); a plain column index uses
// positive positions. The connected user owns the schema, so the USER_ catalog is
// correctly scoped.
func dm8IndexIsFunctionBased(db *gorm.DB, index string) (bool, error) {
	var count int64
	err := db.Raw("SELECT COUNT(*) FROM USER_IND_COLUMNS WHERE INDEX_NAME = ? AND COLUMN_POSITION < 0", index).Scan(&count).Error
	if err != nil {
		return false, fmt.Errorf("dm8 inspect index columns for %s: %w", index, err)
	}
	return count > 0, nil
}
