package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"modernc.org/sqlite"

	"github.com/studoo-app/watchoo/internal/model"
)

// newTestStore ouvre une base temporaire (dans t.TempDir()), applique les migrations et la ferme à la
// fin du test.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "watchoo.db"))
	if err != nil {
		t.Fatalf("Open : %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close : %v", err)
		}
	})
	if err := s.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate : %v", err)
	}
	return s
}

// sqliteCode renvoie le code d'erreur SQLite étendu de err, ou 0 si err n'est pas une erreur SQLite.
func sqliteCode(err error) int {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code()
	}
	return 0
}

// mustExec exécute une requête qui doit réussir et renvoie le dernier identifiant inséré.
func mustExec(t *testing.T, s *Store, query string, args ...any) int64 {
	t.Helper()
	res, err := s.DB().ExecContext(t.Context(), query, args...)
	if err != nil {
		t.Fatalf("requête %q : %v", query, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId : %v", err)
	}
	return id
}

// execErr exécute une requête qui doit échouer et renvoie son erreur.
func execErr(t *testing.T, s *Store, query string, args ...any) error {
	t.Helper()
	_, err := s.DB().ExecContext(t.Context(), query, args...)
	if err == nil {
		t.Fatalf("requête %q : erreur attendue, aucune reçue", query)
	}
	return err
}

// count renvoie le résultat d'une requête « SELECT count(*) … ».
func count(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.DB().QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("requête %q : %v", query, err)
	}
	return n
}

// testRepo renvoie un dépôt factice minimal, distinct pour chaque providerID.
func testRepo(providerID int64, owner, name string) model.Repository {
	return model.Repository{
		Provider:   model.ProviderGitHub,
		ProviderID: providerID,
		Owner:      owner,
		Name:       name,
		Enabled:    true,
	}
}

// mustCreateRepo ajoute un dépôt factice et renvoie son identifiant interne.
func mustCreateRepo(t *testing.T, s *Store, providerID int64, owner, name string) int64 {
	t.Helper()
	r, err := s.CreateRepository(t.Context(), testRepo(providerID, owner, name))
	if err != nil {
		t.Fatalf("CreateRepository(%s/%s) : %v", owner, name, err)
	}
	return r.ID
}

// tableNames renvoie les tables de la base, hors tables internes de SQLite et table de version goose.
func tableNames(ctx context.Context, t *testing.T, sqlDB *sql.DB) map[string]bool {
	t.Helper()
	rows, err := sqlDB.QueryContext(ctx, `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' AND name <> 'goose_db_version'`)
	if err != nil {
		t.Fatalf("liste des tables : %v", err)
	}
	defer rows.Close()
	names := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("lecture d'un nom de table : %v", err)
		}
		names[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("liste des tables : %v", err)
	}
	return names
}
