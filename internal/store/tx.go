package store

// Transactions d'écriture et de lecture du Store.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/studoo-app/watchoo/internal/store/db"
)

// writeTx exécute fn dans une transaction d'écriture « BEGIN IMMEDIATE » sur une connexion dédiée :
// le verrou d'écriture de SQLite est pris dès le début (en attendant au plus busy_timeout), si bien que
// deux processus (serve et tui sur la même base) ne peuvent pas lire puis écrire en même temps. Une
// transaction ordinaire (BEGIN DEFERRED) pourrait au contraire échouer en SQLITE_BUSY au moment de passer
// de la lecture à l'écriture, sans que busy_timeout ne s'applique.
//
// database/sql ne sait pas ouvrir une transaction IMMEDIATE : la transaction est donc pilotée à la main
// sur une *sql.Conn. Si fn renvoie une erreur, tout est annulé (ROLLBACK) ; si l'annulation elle-même
// échoue, la connexion est écartée du pool pour ne jamais rendre une transaction ouverte.
func (s *Store) writeTx(ctx context.Context, fn func(q *db.Queries) error) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("connexion à la base : %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("début de transaction : %w", err)
	}
	// ROLLBACK et COMMIT ne doivent pas être empêchés par l'annulation du contexte de l'appelant.
	finishCtx := context.WithoutCancel(ctx)
	committed := false
	defer func() {
		// Aussi en cas de panique dans fn : la connexion ne retourne jamais au pool en transaction.
		if committed {
			return
		}
		if _, rbErr := conn.ExecContext(finishCtx, "ROLLBACK"); rbErr != nil {
			discardConn(conn)
		}
	}()

	if err := fn(db.New(conn)); err != nil {
		return err
	}
	if _, err := conn.ExecContext(finishCtx, "COMMIT"); err != nil {
		return fmt.Errorf("validation de la transaction : %w", err)
	}
	committed = true
	return nil
}

// discardConn retire conn du pool de database/sql (driver.ErrBadConn la fait fermer à sa libération).
func discardConn(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
}

// readTx exécute fn dans une transaction de lecture : en mode WAL, toutes les requêtes de fn voient le
// même état de la base, même si une synchro écrit en même temps.
func (s *Store) readTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("début de transaction : %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(s.queries.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("fin de transaction : %w", err)
	}
	return nil
}
