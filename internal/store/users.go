package store

import "context"

type UserInput struct {
	Username, Name, Role string
	StoreID              *int64 // wajib untuk admin, nil untuk master
	Active               bool
}

const userSelectSQL = `
SELECT u.id, u.username, u.name, u.password_hash, u.role, u.store_id, s.name AS store_name,
       u.active, u.created_at
FROM users u
LEFT JOIN stores s ON s.id = u.store_id`

// CreateUser membuat user baru atau, bila username sudah ada, memperbarui data & password-nya
// (dipakai CLI sebagai reset password).
func (s *Store) CreateUser(ctx context.Context, in UserInput, hash string) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx,
		`INSERT INTO users (username, name, password_hash, role, store_id, active) VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (username) DO UPDATE SET name = EXCLUDED.name, password_hash = EXCLUDED.password_hash,
		   role = EXCLUDED.role, store_id = EXCLUDED.store_id, active = EXCLUDED.active
		 RETURNING id`, in.Username, in.Name, hash, in.Role, in.StoreID, in.Active).Scan(&id)
	return id, err
}

// InsertUser membuat user baru; gagal (unique violation) bila username sudah dipakai.
func (s *Store) InsertUser(ctx context.Context, in UserInput, hash string) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx,
		`INSERT INTO users (username, name, password_hash, role, store_id, active)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		in.Username, in.Name, hash, in.Role, in.StoreID, in.Active).Scan(&id)
	return id, err
}

func (s *Store) UpdateUser(ctx context.Context, id int64, in UserInput) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE users SET username = $2, name = $3, role = $4, store_id = $5, active = $6 WHERE id = $1`,
		id, in.Username, in.Name, in.Role, in.StoreID, in.Active)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) SetUserPassword(ctx context.Context, id int64, hash string) error {
	tag, err := s.db.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, id, hash)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	rows, err := s.db.Query(ctx, userSelectSQL+` WHERE u.username = $1`, username)
	return collectOne[User](rows, err)
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	rows, err := s.db.Query(ctx, userSelectSQL+` WHERE u.id = $1`, id)
	return collectOne[User](rows, err)
}

// ListUsers mengembalikan semua user; storeID > 0 membatasi ke admin toko tersebut.
func (s *Store) ListUsers(ctx context.Context, storeID int64) ([]User, error) {
	rows, err := s.db.Query(ctx, userSelectSQL+`
		WHERE ($1::bigint = 0 OR u.store_id = $1)
		ORDER BY u.role DESC, s.name NULLS FIRST, u.username`, storeID)
	return collect[User](rows, err)
}
