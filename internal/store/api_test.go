package store

import (
	"reflect"
	"testing"

	"github.com/studoo-app/watchoo/internal/store/db"
)

// TestPublicAPIHidesGeneratedTypes vérifie la règle d'architecture « les types générés par sqlc ne
// sortent pas de internal/store » : aucune méthode exportée de *Store, ni aucune fonction exportée du
// package (liste ci-dessous, à compléter), ne reçoit ou ne renvoie un type du package db (même dans un
// pointeur, une liste…).
func TestPublicAPIHidesGeneratedTypes(t *testing.T) {
	check := func(name string, fn reflect.Type) {
		for i := range fn.NumIn() {
			if usesGenerated(fn.In(i)) {
				t.Errorf("%s reçoit un type généré par sqlc : %s", name, fn.In(i))
			}
		}
		for i := range fn.NumOut() {
			if usesGenerated(fn.Out(i)) {
				t.Errorf("%s renvoie un type généré par sqlc : %s", name, fn.Out(i))
			}
		}
	}

	st := reflect.TypeFor[*Store]()
	if st.NumMethod() == 0 {
		t.Fatal("aucune méthode exportée trouvée sur *Store")
	}
	for i := range st.NumMethod() {
		m := st.Method(i)
		check("Store."+m.Name, m.Type)
	}
	for name, fn := range map[string]any{
		"Open": Open, "FormatTime": FormatTime, "ParseTime": ParseTime, "CreateDBFile": CreateDBFile,
	} {
		check(name, reflect.TypeOf(fn))
	}
}

// usesGenerated indique si le type t est un type du package db, ou en contient un (pointeur, liste,
// tableau, map, canal).
func usesGenerated(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Chan:
		return usesGenerated(t.Elem())
	case reflect.Map:
		return usesGenerated(t.Key()) || usesGenerated(t.Elem())
	}
	return t.PkgPath() == reflect.TypeFor[db.Queries]().PkgPath()
}
