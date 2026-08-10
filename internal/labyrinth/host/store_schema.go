package host

import "github.com/jackc/pgx/v5"

func (store *Store) relation(name string) string {
	return qualifiedHostRelation(store.schema, name)
}

func qualifiedHostRelation(schema string, name string) string {
	return pgx.Identifier{schema, name}.Sanitize()
}
