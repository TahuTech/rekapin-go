// Package store berisi akses data ke PostgreSQL per domain.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound dikembalikan saat baris tidak ditemukan.
var ErrNotFound = errors.New("data tidak ditemukan")

// DBTX dipenuhi oleh *pgxpool.Pool maupun pgx.Tx sehingga query yang sama bisa
// dipakai di dalam atau di luar transaksi.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Store struct {
	db   DBTX
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{db: pool, pool: pool}
}

// WithTx menjalankan fn di dalam satu transaksi; rollback otomatis bila fn error.
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&Store{db: tx, pool: s.pool})
	})
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// collect men-scan semua baris ke slice T berdasarkan tag `db`.
func collect[T any](rows pgx.Rows, err error) ([]T, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByNameLax[T])
}

// collectOne men-scan tepat satu baris.
func collectOne[T any](rows pgx.Rows, err error) (T, error) {
	var zero T
	if err != nil {
		return zero, err
	}
	v, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[T])
	return v, notFound(err)
}

// Page adalah parameter paginasi offset sederhana.
type Page struct {
	Num  int // mulai dari 1
	Size int
}

func (p Page) Limit() int {
	if p.Size <= 0 {
		return 25
	}
	return p.Size
}

func (p Page) Offset() int {
	if p.Num <= 1 {
		return 0
	}
	return (p.Num - 1) * p.Limit()
}
