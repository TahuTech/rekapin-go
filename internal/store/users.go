package store

import "context"

func (s *Store) CreateUser(ctx context.Context, username, name, hash string) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx,
		`INSERT INTO users (username, name, password_hash) VALUES ($1, $2, $3)
		 ON CONFLICT (username) DO UPDATE SET name = EXCLUDED.name, password_hash = EXCLUDED.password_hash
		 RETURNING id`, username, name, hash).Scan(&id)
	return id, err
}

func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	rows, err := s.db.Query(ctx, `SELECT * FROM users WHERE username = $1`, username)
	return collectOne[User](rows, err)
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	rows, err := s.db.Query(ctx, `SELECT * FROM users WHERE id = $1`, id)
	return collectOne[User](rows, err)
}
