package store

import "context"

type CustomerInput struct {
	Name, Phone, Address, Notes string
}

// customerSummarySQL menggabungkan customer dengan agregat saldo transaksinya.
const customerSummarySQL = `
SELECT c.*,
       COUNT(b.order_id)              AS order_count,
       COALESCE(SUM(b.total), 0)::BIGINT     AS total_buy,
       COALESCE(SUM(b.paid), 0)::BIGINT      AS total_paid,
       COALESCE(SUM(b.remaining), 0)::BIGINT AS remaining
FROM customers c
LEFT JOIN order_balances b ON b.customer_id = c.id`

func (s *Store) ListCustomers(ctx context.Context, q string, p Page) ([]CustomerSummary, int, error) {
	var total int
	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM customers WHERE store_id = $2 AND archived_at IS NULL
		 AND ($1 = '' OR name ILIKE '%' || $1 || '%' OR phone ILIKE '%' || $1 || '%')`, q, s.sid()).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(ctx, customerSummarySQL+`
		WHERE c.store_id = $4 AND c.archived_at IS NULL
		  AND ($1 = '' OR c.name ILIKE '%' || $1 || '%' OR c.phone ILIKE '%' || $1 || '%')
		GROUP BY c.id ORDER BY c.name LIMIT $2 OFFSET $3`, q, p.Limit(), p.Offset(), s.sid())
	list, err := collect[CustomerSummary](rows, err)
	return list, total, err
}

// CustomerOptions untuk dropdown (ringan, tanpa agregat).
func (s *Store) CustomerOptions(ctx context.Context) ([]Customer, error) {
	rows, err := s.db.Query(ctx, `SELECT * FROM customers WHERE store_id = $1 AND archived_at IS NULL ORDER BY name`, s.sid())
	return collect[Customer](rows, err)
}

func (s *Store) CustomerSummaryByID(ctx context.Context, id int64) (CustomerSummary, error) {
	rows, err := s.db.Query(ctx, customerSummarySQL+` WHERE c.id = $1 AND c.store_id = $2 GROUP BY c.id`, id, s.sid())
	return collectOne[CustomerSummary](rows, err)
}

func (s *Store) CustomerByID(ctx context.Context, id int64) (Customer, error) {
	rows, err := s.db.Query(ctx, `SELECT * FROM customers WHERE id = $1 AND store_id = $2`, id, s.sid())
	return collectOne[Customer](rows, err)
}

func (s *Store) CreateCustomer(ctx context.Context, in CustomerInput) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx,
		`INSERT INTO customers (store_id, name, phone, address, notes) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		s.sid(), in.Name, in.Phone, in.Address, in.Notes).Scan(&id)
	return id, err
}

func (s *Store) UpdateCustomer(ctx context.Context, id int64, in CustomerInput) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE customers SET name = $2, phone = $3, address = $4, notes = $5 WHERE id = $1 AND store_id = $6`,
		id, in.Name, in.Phone, in.Address, in.Notes, s.sid())
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ArchiveCustomer menyembunyikan customer tanpa menghapus histori transaksinya.
func (s *Store) ArchiveCustomer(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, `UPDATE customers SET archived_at = now() WHERE id = $1 AND store_id = $2`, id, s.sid())
	return err
}
