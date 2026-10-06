package store

import "context"

type ProductInput struct {
	Name, Unit string
	Price      int64
	Active     bool
}

func (s *Store) ListProducts(ctx context.Context, q string, onlyActive bool) ([]Product, error) {
	rows, err := s.db.Query(ctx,
		`SELECT * FROM products
		 WHERE ($1 = '' OR name ILIKE '%' || $1 || '%') AND (NOT $2 OR active)
		 ORDER BY active DESC, name`, q, onlyActive)
	return collect[Product](rows, err)
}

func (s *Store) ProductByID(ctx context.Context, id int64) (Product, error) {
	rows, err := s.db.Query(ctx, `SELECT * FROM products WHERE id = $1`, id)
	return collectOne[Product](rows, err)
}

func (s *Store) CreateProduct(ctx context.Context, in ProductInput) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx,
		`INSERT INTO products (name, unit, price, active) VALUES ($1, $2, $3, $4) RETURNING id`,
		in.Name, in.Unit, in.Price, in.Active).Scan(&id)
	return id, err
}

func (s *Store) UpdateProduct(ctx context.Context, id int64, in ProductInput) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE products SET name = $2, unit = $3, price = $4, active = $5 WHERE id = $1`,
		id, in.Name, in.Unit, in.Price, in.Active)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) ToggleProduct(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, `UPDATE products SET active = NOT active WHERE id = $1`, id)
	return err
}
