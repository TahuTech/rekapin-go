package store

import (
	"context"
	"time"
)

type PaymentInput struct {
	OrderID   int64
	Amount    int64
	PaidAt    time.Time
	Method    string
	Note      string
	InvoiceID *int64
}

type PaymentFilter struct {
	CustomerID int64
	OrderID    int64
	Method     string
	ShowVoided bool
	DateRange
}

const paymentSelectSQL = `
SELECT p.id, p.order_id, o.code AS order_code, o.customer_id, c.name AS customer_name,
       p.amount, p.paid_at, p.method, p.note, p.invoice_id, i.number AS invoice_number,
       u.name AS created_by_name, p.created_at, p.voided_at
FROM payments p
JOIN orders o ON o.id = p.order_id
JOIN customers c ON c.id = o.customer_id
LEFT JOIN invoices i ON i.id = p.invoice_id
LEFT JOIN users u ON u.id = p.created_by`

const paymentWhereSQL = `
WHERE ($1::bigint = 0 OR o.customer_id = $1)
  AND ($2::bigint = 0 OR p.order_id = $2)
  AND ($3 = '' OR p.method = $3)
  AND ($4 OR p.voided_at IS NULL)
  AND ($5::date IS NULL OR p.paid_at >= $5)
  AND ($6::date IS NULL OR p.paid_at <= $6)`

func (f PaymentFilter) args() []any {
	return []any{f.CustomerID, f.OrderID, f.Method, f.ShowVoided, f.From, f.To}
}

// ListPayments mengembalikan log pembayaran beserta total nominal (non-void) sesuai filter.
func (s *Store) ListPayments(ctx context.Context, f PaymentFilter, p Page) ([]Payment, int, int64, error) {
	var count int
	var sum int64
	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(p.amount) FILTER (WHERE p.voided_at IS NULL), 0)::BIGINT
		 FROM payments p JOIN orders o ON o.id = p.order_id`+paymentWhereSQL, f.args()...).Scan(&count, &sum); err != nil {
		return nil, 0, 0, err
	}
	args := append(f.args(), p.Limit(), p.Offset())
	rows, err := s.db.Query(ctx, paymentSelectSQL+paymentWhereSQL+
		` ORDER BY p.paid_at DESC, p.id DESC LIMIT $7 OFFSET $8`, args...)
	list, err := collect[Payment](rows, err)
	return list, count, sum, err
}

func (s *Store) PaymentByID(ctx context.Context, id int64) (Payment, error) {
	rows, err := s.db.Query(ctx, paymentSelectSQL+` WHERE p.id = $1`, id)
	return collectOne[Payment](rows, err)
}

func (s *Store) InsertPayment(ctx context.Context, in PaymentInput, userID int64) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx,
		`INSERT INTO payments (order_id, amount, paid_at, method, note, invoice_id, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		in.OrderID, in.Amount, in.PaidAt, in.Method, in.Note, in.InvoiceID, userID).Scan(&id)
	return id, err
}

func (s *Store) VoidPayment(ctx context.Context, id, userID int64) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE payments SET voided_at = now(), voided_by = $2 WHERE id = $1 AND voided_at IS NULL`, id, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
