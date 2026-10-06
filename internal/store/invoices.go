package store

import (
	"context"
	"time"
)

type InvoiceInput struct {
	Type       string // tagihan | pelunasan
	CustomerID int64
	IssueDate  time.Time
	DueDate    *time.Time
	Notes      string
}

type InvoiceFilter struct {
	CustomerID int64
	Type       string
	DateRange
}

const invoiceSelectSQL = `
SELECT inv.id, inv.number, inv.type, inv.customer_id, c.name AS customer_name,
       c.phone AS customer_phone, c.address AS customer_address,
       inv.issue_date, inv.due_date, inv.notes, inv.created_at, u.name AS created_by_name,
       COALESCE((SELECT SUM(amount_due) FROM invoice_orders io WHERE io.invoice_id = inv.id), 0)::BIGINT AS total
FROM invoices inv
JOIN customers c ON c.id = inv.customer_id
LEFT JOIN users u ON u.id = inv.created_by`

func (s *Store) ListInvoices(ctx context.Context, f InvoiceFilter, p Page) ([]Invoice, int, error) {
	const where = `
		WHERE ($1::bigint = 0 OR inv.customer_id = $1)
		  AND ($2 = '' OR inv.type = $2)
		  AND ($3::date IS NULL OR inv.issue_date >= $3)
		  AND ($4::date IS NULL OR inv.issue_date <= $4)
		  AND inv.store_id = $5`
	args := []any{f.CustomerID, f.Type, f.From, f.To, s.sid()}
	var total int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM invoices inv`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(ctx, invoiceSelectSQL+where+` ORDER BY inv.issue_date DESC, inv.id DESC LIMIT $6 OFFSET $7`,
		append(args, p.Limit(), p.Offset())...)
	list, err := collect[Invoice](rows, err)
	return list, total, err
}

func (s *Store) InvoiceByID(ctx context.Context, id int64) (Invoice, error) {
	rows, err := s.db.Query(ctx, invoiceSelectSQL+` WHERE inv.id = $1 AND inv.store_id = $2`, id, s.sid())
	return collectOne[Invoice](rows, err)
}

func (s *Store) InvoiceLines(ctx context.Context, invoiceID int64) ([]InvoiceLine, error) {
	rows, err := s.db.Query(ctx, `
		SELECT io.order_id, o.code AS order_code, o.order_date, o.total AS order_total,
		       (o.total - io.amount_due) AS paid_before, io.amount_due,
		       COALESCE((SELECT string_agg(i.product_name || ' ×' || trim_scale(i.qty)::text, ', ' ORDER BY i.id)
		                 FROM order_items i WHERE i.order_id = o.id), '') AS items
		FROM invoice_orders io JOIN orders o ON o.id = io.order_id
		WHERE io.invoice_id = $1 AND o.store_id = $2 ORDER BY o.order_date, o.id`, invoiceID, s.sid())
	return collect[InvoiceLine](rows, err)
}

func (s *Store) InsertInvoice(ctx context.Context, number string, in InvoiceInput, userID int64) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx,
		`INSERT INTO invoices (store_id, number, type, customer_id, issue_date, due_date, notes, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		s.sid(), number, in.Type, in.CustomerID, in.IssueDate, in.DueDate, in.Notes, userID).Scan(&id)
	return id, err
}

// InsertInvoiceOrder hanya menautkan invoice & order milik toko aktif.
func (s *Store) InsertInvoiceOrder(ctx context.Context, invoiceID, orderID, amountDue int64) error {
	tag, err := s.db.Exec(ctx,
		`INSERT INTO invoice_orders (invoice_id, order_id, amount_due)
		 SELECT i.id, o.id, $3::bigint FROM invoices i, orders o
		 WHERE i.id = $1 AND o.id = $2 AND i.store_id = $4 AND o.store_id = $4`,
		invoiceID, orderID, amountDue, s.sid())
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// InvoicePayments = pembayaran yang tercatat oleh invoice pelunasan.
func (s *Store) InvoicePayments(ctx context.Context, invoiceID int64) ([]Payment, error) {
	rows, err := s.db.Query(ctx, paymentSelectSQL+` WHERE p.invoice_id = $1 AND p.store_id = $2 ORDER BY p.id`, invoiceID, s.sid())
	return collect[Payment](rows, err)
}
