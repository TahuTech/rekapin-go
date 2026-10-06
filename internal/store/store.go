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
	db      DBTX
	pool    *pgxpool.Pool
	storeID int64 // toko aktif; 0 = belum di-scope (hanya untuk query user/toko)
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{db: pool, pool: pool}
}

// ForStore mengembalikan Store yang semua query domainnya dibatasi ke satu toko.
func (s *Store) ForStore(id int64) *Store {
	return &Store{db: s.db, pool: s.pool, storeID: id}
}

// StoreID = toko aktif dari Store ter-scope.
func (s *Store) StoreID() int64 { return s.storeID }

// sid dipakai semua query data toko. Panic bila Store belum di-scope agar bug
// gagal keras alih-alih membocorkan/menulis data lintas toko.
func (s *Store) sid() int64 {
	if s.storeID == 0 {
		panic("store: query data toko tanpa ForStore")
	}
	return s.storeID
}

// WithTx menjalankan fn di dalam satu transaksi; rollback otomatis bila fn error.
// Scope toko ikut diwariskan ke Store transaksi.
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&Store{db: tx, pool: s.pool, storeID: s.storeID})
	})
}

// IsUniqueViolation true bila err berasal dari pelanggaran constraint UNIQUE.
func IsUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
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
