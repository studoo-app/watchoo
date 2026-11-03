package store

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/studoo-app/watchoo/internal/store/migrations"
)

// expectedTables are the 13 tables of the data model (documentation/en/04-data-model.md): the 7
// of the MVP, the 3 of V2 (migration 00003), the languages (migration 00007), the AI signatures
// (migration 00008) and the languages of the commits (migration 00009).
var expectedTables = []string{
	"repositories", "branches", "contributors", "contributor_emails", "commits", "commit_branches",
	"sync_runs", "pull_requests", "releases", "tags", "repository_languages", "commit_ai_signatures",
	"commit_languages",
}

// expectedIndexes are the indexes created explicitly by the migrations.
var expectedIndexes = []string{
	"pull_requests_repo_id_updated_at",
	"commits_repo_id_committed_at",
	"commits_author_id_committed_at",
	"commit_branches_branch_id",
	"contributor_emails_contributor_id",
	"sync_runs_repo_id_started_at",
}

func TestDSN(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/tmp/watchoo.db", "file:///tmp/watchoo.db"},
		{"rel/watchoo.db", "file:rel/watchoo.db"},
		{"/tmp/a b/watchoo.db", "file:///tmp/a%20b/watchoo.db"},
		{"/tmp/x?y#z/watchoo.db", "file:///tmp/x%3Fy%23z/watchoo.db"},
		// A "%" is always escaped, even when followed by hexadecimal digits.
		{"/tmp/100%25/watchoo.db", "file:///tmp/100%2525/watchoo.db"},
		// A path starting with "//" must not be read as a host name.
		{"//tmp/watchoo.db", "file:////tmp/watchoo.db"},
	}
	for _, tt := range tests {
		if got, want := dsn(tt.path), tt.want+"?"+DSNPragmas; got != want {
			t.Errorf("dsn(%q) = %q, want %q", tt.path, got, want)
		}
	}
}

func TestOpenCreatesFilesWithMode0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watchoo.db")
	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if err := s.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// A write guarantees that the WAL journal and the shared memory exist.
	mustCreateRepo(t, s, 1, "promo", "alice")

	for _, name := range []string{"watchoo.db", "watchoo.db-wal", "watchoo.db-shm"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("Stat(%s): %v", name, err)
		}
		if got := info.Mode().Perm(); got != FileMode {
			t.Errorf("%s: permissions %o, want %o", name, got, FileMode)
		}
	}
}

func TestOpenPragmasOnEveryConnection(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	// Several connections open at the same time: each one must have received the pragmas of the DSN.
	for i := range 3 {
		conn, err := s.DB().Conn(ctx)
		if err != nil {
			t.Fatalf("Conn: %v", err)
		}
		defer conn.Close()

		var foreignKeys, busyTimeout int
		var journalMode string
		if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
			t.Fatalf("PRAGMA foreign_keys: %v", err)
		}
		if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
			t.Fatalf("PRAGMA busy_timeout: %v", err)
		}
		if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
			t.Fatalf("PRAGMA journal_mode: %v", err)
		}
		if foreignKeys != 1 || busyTimeout != 5000 || journalMode != "wal" {
			t.Errorf("connection %d: foreign_keys=%d busy_timeout=%d journal_mode=%q, want 1, 5000, \"wal\"",
				i, foreignKeys, busyTimeout, journalMode)
		}
	}
}

func TestOpenEscapedPath(t *testing.T) {
	parent := t.TempDir()
	// Space, "#", "?" and "%" followed by digits: nothing must be interpreted by the SQLite URI.
	dir := filepath.Join(parent, "promo 2026 #3 ?x %20")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	path := filepath.Join(dir, "watchoo.db")

	s, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mustCreateRepo(t, s, 1, "promo", "alice")

	// The database is the one of the requested path, and no other file was created.
	var got []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if want := []string{"watchoo.db", "watchoo.db-shm", "watchoo.db-wal"}; !slices.Equal(got, want) {
		t.Errorf("files of %q = %v, want %v", dir, got, want)
	}
	parentEntries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(parentEntries) != 1 {
		t.Errorf("unexpected files next to the database directory: %v", parentEntries)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopening: the data is found again.
	s, err = Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open (reopening): %v", err)
	}
	defer s.Close()
	repos, err := s.ListRepositories(t.Context())
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 1 || repos[0].FullName() != "promo/alice" {
		t.Errorf("repositories after reopening = %+v, want promo/alice", repos)
	}
}

func TestOpenErrorMentionsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent", "watchoo.db")
	s, err := Open(t.Context(), path)
	if err == nil {
		s.Close()
		t.Fatal("Open in a nonexistent directory: error expected")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q: the path %q should appear in it", err, path)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error %q: should wrap fs.ErrNotExist", err)
	}
}

func TestOpenEmptyPath(t *testing.T) {
	if s, err := Open(t.Context(), ""); err == nil {
		s.Close()
		t.Fatal("Open(\"\"): error expected")
	}
}

func TestCreateDBFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchoo.db")
	if err := CreateDBFile(path); err != nil {
		t.Fatalf("CreateDBFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != FileMode {
		t.Errorf("permissions %o, want %o", got, FileMode)
	}
	if info.Size() != 0 {
		t.Errorf("size %d, want an empty file", info.Size())
	}
}

func TestCreateDBFileKeepsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchoo.db")
	content := []byte("contenu existant")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	if err := CreateDBFile(path); err != nil {
		t.Fatalf("CreateDBFile on an existing file: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content changed: %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o644 {
		t.Errorf("permissions changed: %o, want 644", mode)
	}
}

func TestCreateDBFileErrors(t *testing.T) {
	dir := t.TempDir()
	if err := CreateDBFile(dir); err == nil {
		t.Error("CreateDBFile on a directory: error expected")
	}
	missing := filepath.Join(dir, "absent", "watchoo.db")
	if err := CreateDBFile(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("CreateDBFile without a parent directory: error %v, want fs.ErrNotExist", err)
	}
}

func TestMigrateCreatesTablesAndIndexes(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	tables := tableNames(ctx, t, s.DB())
	if len(tables) != len(expectedTables) {
		t.Errorf("tables = %v, want %v", tables, expectedTables)
	}
	for _, name := range expectedTables {
		if !tables[name] {
			t.Errorf("table %s missing", name)
		}
	}

	for _, name := range expectedIndexes {
		n := count(t, s, "SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?", name)
		if n != 1 {
			t.Errorf("index %s missing", name)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := newTestStore(t) // first migration
	ctx := t.Context()
	mustCreateRepo(t, s, 1, "promo", "alice")

	for i := range 2 {
		if err := s.Migrate(ctx); err != nil {
			t.Fatalf("Migrate (extra call %d): %v", i+1, err)
		}
	}

	provider, err := goose.NewProvider(MigrationDialect, s.DB(), migrations.FS,
		goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	current, target, err := provider.GetVersions(ctx)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if current != target || current < 1 {
		t.Errorf("schema version %d, last migration %d", current, target)
	}
	if n := count(t, s, "SELECT count(*) FROM repositories"); n != 1 {
		t.Errorf("%d repositories after repeated migrations, want 1", n)
	}
}

func TestMigrateAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "watchoo.db")
	for i := range 2 {
		s, err := Open(t.Context(), path)
		if err != nil {
			t.Fatalf("Open %d: %v", i, err)
		}
		if err := s.Migrate(t.Context()); err != nil {
			t.Fatalf("Migrate %d: %v", i, err)
		}
		if i == 0 {
			mustCreateRepo(t, s, 1, "promo", "alice")
		} else if n := count(t, s, "SELECT count(*) FROM repositories"); n != 1 {
			t.Errorf("%d repositories after reopening, want 1", n)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("Close %d: %v", i, err)
		}
	}
}

func TestMigrationsDown(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	mustCreateRepo(t, s, 1, "promo", "alice")

	provider, err := goose.NewProvider(MigrationDialect, s.DB(), migrations.FS,
		goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.DownTo(ctx, 0); err != nil {
		t.Fatalf("DownTo(0): %v", err)
	}
	if tables := tableNames(ctx, t, s.DB()); len(tables) != 0 {
		t.Errorf("tables left after the rollback: %v", tables)
	}
	if n := count(t, s, "SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name NOT LIKE 'sqlite_%'"); n != 0 {
		t.Errorf("%d indexes left after the rollback", n)
	}

	// The migrations are then applied again normally.
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate after the rollback: %v", err)
	}
	if tables := tableNames(ctx, t, s.DB()); len(tables) != len(expectedTables) {
		t.Errorf("tables after the new migration: %v", tables)
	}
}

func TestConcurrentWrites(t *testing.T) {
	s := newTestStore(t)
	const writers = 8

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		wg.Go(func() {
			_, err := s.CreateRepository(t.Context(), testRepo(int64(i+1), "promo", fmt.Sprintf("etudiant-%d", i)))
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent write: %v", err)
		}
	}
	if n := count(t, s, "SELECT count(*) FROM repositories"); n != writers {
		t.Errorf("%d repositories, want %d", n, writers)
	}
}
