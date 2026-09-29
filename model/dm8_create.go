package model

import (
	"errors"
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
)

// registerDM8Create replaces the create callback on a DM8 connection so that
// clause.OnConflict on a NON-primary-key unique target behaves correctly.
//
// The fixed community driver only builds a MERGE when every primary-key field is
// present in the insert; otherwise it emits a plain INSERT. new-api relies on
// OnConflict against non-PK unique columns (auth flow token_hash, authz role key,
// system_instance node_name, perf_metric composite, external identity claims),
// which a plain INSERT turns into a uniqueness error instead of an
// ignore/upsert.
//
// The driver's own path is preserved: original runs first, so primary-key MERGE
// and conflict-free inserts are untouched. Only a DM8 uniqueness error from the
// plain INSERT is intercepted and resolved here. DM8 keeps a transaction usable
// after a statement error (verified), so the fallback runs inside the caller's transaction.
func registerDM8Create(db *gorm.DB) error {
	if db.Dialector.Name() != "dm" {
		return fmt.Errorf("DM create adapter received %s", db.Dialector.Name())
	}
	original := db.Callback().Create().Get("gorm:create")
	if original == nil {
		return errors.New("DM create callback is missing")
	}
	return db.Callback().Create().Replace("gorm:create", func(tx *gorm.DB) {
		conflictClause, present := tx.Statement.Clauses["ON CONFLICT"]
		if !present {
			original(tx)
			return
		}
		onConflict, ok := conflictClause.Expression.(clause.OnConflict)
		if !ok {
			original(tx)
			return
		}
		original(tx)
		if tx.Error == nil || !isDM8UniqueViolation(tx.Error) {
			// Conflict-free insert, driver-handled primary-key MERGE, or a genuine
			// (non-uniqueness) error: leave the driver's result as-is.
			return
		}
		if len(onConflict.DoUpdates) == 0 {
			createDM8ResolveDoNothing(tx)
			return
		}
		createDM8ResolveDoUpdates(tx, onConflict)
	})
}

// createDM8ResolveDoNothing honors OnConflict{DoNothing} after the plain INSERT hit
// a uniqueness violation. A single row becomes a no-op with RowsAffected=0 (the
// contract auth flow and identity claims depend on). A batch INSERT is atomic in
// DM8, so nothing was written; it is replayed row-by-row, skipping rows that
// conflict and backfilling identities for rows that insert.
func createDM8ResolveDoNothing(tx *gorm.DB) {
	reflectValue := tx.Statement.ReflectValue
	if reflectValue.Kind() != reflect.Slice && reflectValue.Kind() != reflect.Array {
		tx.Error = nil
		tx.RowsAffected = 0
		return
	}
	tx.Error = nil
	tx.RowsAffected = 0
	for i := 0; i < reflectValue.Len(); i++ {
		element := reflectValue.Index(i)
		var dest any
		if element.Kind() == reflect.Pointer {
			if element.IsNil() {
				continue
			}
			dest = element.Interface()
		} else {
			if !element.CanAddr() {
				tx.Error = errors.New("DM8 batch conflict handling requires an addressable slice")
				return
			}
			dest = element.Addr().Interface()
		}
		// A fresh session drops the ON CONFLICT clause, so this re-enters the
		// driver's plain INSERT path and backfills the identity into dest.
		sub := tx.Session(&gorm.Session{NewDB: true})
		if err := sub.Create(dest).Error; err != nil {
			if isDM8UniqueViolation(err) {
				continue
			}
			tx.Error = err
			return
		}
		tx.RowsAffected += sub.RowsAffected
	}
}

// createDM8ResolveDoUpdates performs the upsert's UPDATE branch after the INSERT
// conflicted. All DoUpdates call sites upsert a single row, so the assignments are
// applied with a WHERE built from the conflict columns. AssignmentColumns values
// (clause.Column{Table:"excluded"}) are substituted with the row's actual insert
// values; expression assignments (for example additive counters) pass through.
func createDM8ResolveDoUpdates(tx *gorm.DB, onConflict clause.OnConflict) {
	if len(onConflict.Columns) == 0 {
		tx.Error = errors.New("DM8 upsert requires explicit conflict columns")
		return
	}
	values := callbacks.ConvertToCreateValues(tx.Statement)
	if len(values.Values) != 1 {
		tx.Error = fmt.Errorf("DM8 upsert supports a single row, got %d", len(values.Values))
		return
	}
	inserted := make(map[string]any, len(values.Columns))
	for idx, column := range values.Columns {
		inserted[column.Name] = values.Values[0][idx]
	}

	where := make(map[string]any, len(onConflict.Columns))
	for _, column := range onConflict.Columns {
		value, ok := inserted[column.Name]
		if !ok {
			tx.Error = fmt.Errorf("DM8 upsert conflict column %q is not part of the insert", column.Name)
			return
		}
		where[column.Name] = value
	}

	assignments := make(map[string]any, len(onConflict.DoUpdates))
	for _, assignment := range onConflict.DoUpdates {
		name := assignment.Column.Name
		value := assignment.Value
		if ref, ok := value.(clause.Column); ok && ref.Table == "excluded" {
			resolved, found := inserted[ref.Name]
			if !found {
				tx.Error = fmt.Errorf("DM8 upsert cannot resolve excluded column %q", ref.Name)
				return
			}
			value = resolved
		}
		assignments[name] = value
	}
	if len(assignments) == 0 {
		// Nothing to update; the conflict is effectively ignored.
		tx.Error = nil
		tx.RowsAffected = 0
		return
	}

	// The failed INSERT left tx.Error set; Session copies it and would short-circuit
	// the follow-up UPDATE, so clear it first and re-assign from the UPDATE result.
	tx.Error = nil
	result := tx.Session(&gorm.Session{NewDB: true}).
		Table(tx.Statement.Table).
		Where(where).
		Updates(assignments)
	tx.Error = result.Error
	if result.Error == nil {
		tx.RowsAffected = result.RowsAffected
	}
}
