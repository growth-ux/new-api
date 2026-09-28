package model

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// quoteModelIdentifier quotes a model-controlled SQL identifier using the
// active dialect. It is for identifiers in SQL expressions that GORM otherwise
// embeds verbatim; request input must never be passed as an identifier.
func quoteModelIdentifier(db *gorm.DB, name string) string {
	return (&gorm.Statement{DB: db}).Quote(clause.Column{Name: name})
}

// QuoteSQLIdentifier quotes an application-controlled identifier in a raw SQL
// expression. Never pass request data as name; bind request data as a value.
func QuoteSQLIdentifier(db *gorm.DB, name string) string {
	return quoteModelIdentifier(db, name)
}

func quoteModelIdentifiers(db *gorm.DB, names ...string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = quoteModelIdentifier(db, name)
	}
	return strings.Join(quoted, ", ")
}
