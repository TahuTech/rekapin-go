package store

import (
	"context"
	"fmt"
	"time"
)

// DateRange filter tanggal inklusif; nil berarti tidak dibatasi.
type DateRange struct {
	From, To *time.Time
}

type OrderFilter struct {
	CustomerID int64
	Status     string // "", belum, sebagian, lunas, outstanding (belum+sebagian)
	Q          string // cari kode transaksi
	DateRange
}

type OrderItemInput struct {
	ProductID   *int64
	ProductName string
	Unit        string
	Qty         float64
	UnitPrice   int64
}

type OrderInput struct {
	CustomerID int64
	OrderDate  time.Time
	Notes      string
	Items      []OrderItemInput
}

const orderSelectSQL = `
SELECT o.id, o.code, o.customer_id, c.name AS customer_name, o.order_date, o.notes,
       o.total, b.paid, b.remaining, b.status, o.created_at,
       COALESCE((SELECT string_agg(i.product_name || ' ×' || trim_scale(i.qty)::text, ', ' ORDER BY i.id)
                 FROM order_items i WHERE i.order_id = o.id), '') AS items_summary
FROM orders o
JOIN customers c ON c.id = o.customer_id
JOIN order_balances b ON b.order_id = o.id`

const orderWhereSQL = `
WHERE ($1::bigint = 0 OR o.customer_id = $1)
  AND ($2::date IS NULL OR o.order_date >= $2)
  AND ($3::date IS NULL OR o.order_date <= $3)
  AND ($4 = '' OR b.status = $4 OR ($4 = 'outstanding' AND b.status <> 'lunas'))
  AND ($5 = '' OR o.code ILIKE '%' || $5 || '%')
  AND o.store_id = $6`

func (f OrderFilter) args(storeID int64) []any {
	return []any{f.CustomerID, f.From, f.To, f.Status, f.Q, storeID}
}

func (s *Store) ListOrders(ctx context.Context, f OrderFilter, p Page) ([]Order, int, error) {
	var total int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM orders o JOIN order_balances b ON b.order_id = o.id`+orderWhereSQL,
		f.args(s.sid())...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args := append(f.args(s.sid()), p.Limit(), p.Offset())
	rows, err := s.db.Query(ctx, orderSelectSQL+orderWhereSQL+` ORDER BY o.order_date DESC, o.id DESC LIMIT $7 OFFSET $8`, args...)
	list, err := collect[Order](rows, err)
	return list, total, err
}

// OutstandingOrders = transaksi customer yang masih punya sisa, urut terlama.
func (s *Store) OutstandingOrders(ctx context.Context, customerID int64) ([]Order, error) {
	rows, err := s.db.Query(ctx, orderSelectSQL+`
		WHERE o.customer_id = $1 AND o.store_id = $2 AND b.remaining > 0 ORDER BY o.order_date, o.id`, customerID, s.sid())
	return collect[Order](rows, err)
}

func (s *Store) OrderByID(ctx context.Context, id int64) (Order, error) {
	rows, err := s.db.Query(ctx, orderSelectSQL+` WHERE o.id = $1 AND o.store_id = $2`, id, s.sid())
	return collectOne[Order](rows, err)
}

// LockOrder mengunci baris order (SELECT ... FOR UPDATE) lalu mengembalikan saldonya.
// Wajib dipanggil di dalam WithTx agar validasi sisa bebas race condition.
func (s *Store) LockOrder(ctx context.Context, id int64) (Order, error) {
	var lockedID int64
	if err := s.db.QueryRow(ctx, `SELECT id FROM orders WHERE id = $1 AND store_id = $2 FOR UPDATE`, id, s.sid()).Scan(&lockedID); err != nil {
		return Order{}, notFound(err)
	}
	return s.OrderByID(ctx, id)
}

func (s *Store) OrderItems(ctx context.Context, orderID int64) ([]OrderItem, error) {
	rows, err := s.db.Query(ctx, `
		SELECT i.* FROM order_items i JOIN orders o ON o.id = i.order_id
		WHERE i.order_id = $1 AND o.store_id = $2 ORDER BY i.id`, orderID, s.sid())
	return collect[OrderItem](rows, err)
}

// InsertOrder menyimpan header + item. total dihitung pemanggil.
func (s *Store) InsertOrder(ctx context.Context, code string, in OrderInput, total int64, userID int64) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx,
		`INSERT INTO orders (store_id, code, customer_id, order_date, notes, total, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		s.sid(), code, in.CustomerID, in.OrderDate, in.Notes, total, userID).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, s.insertItems(ctx, id, in.Items)
}

// ReplaceOrder memperbarui header dan mengganti seluruh item.
func (s *Store) ReplaceOrder(ctx context.Context, id int64, in OrderInput, total int64) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE orders SET customer_id = $2, order_date = $3, notes = $4, total = $5 WHERE id = $1 AND store_id = $6`,
		id, in.CustomerID, in.OrderDate, in.Notes, total, s.sid())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM order_items WHERE order_id = $1`, id); err != nil {
		return err
	}
	return s.insertItems(ctx, id, in.Items)
}

func (s *Store) insertItems(ctx context.Context, orderID int64, items []OrderItemInput) error {
	for _, it := range items {
		// product_id hanya disimpan bila produk milik toko yang sama; selain itu NULL
		// (nama & harga tetap tersimpan sebagai snapshot).
		if _, err := s.db.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, product_name, unit, qty, unit_price, subtotal)
			 VALUES ($1, (SELECT id FROM products WHERE id = $2 AND store_id = $8), $3, $4, $5, $6, $7)`,
			orderID, it.ProductID, it.ProductName, it.Unit, it.Qty, it.UnitPrice, LineSubtotal(it.Qty, it.UnitPrice), s.sid()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteOrder(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, `DELETE FROM orders WHERE id = $1 AND store_id = $2`, id, s.sid())
	return err
}

// OrderHasHistory true bila order sudah punya pembayaran (termasuk void) atau masuk invoice.
func (s *Store) OrderHasHistory(ctx context.Context, id int64) (bool, error) {
	var has bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM payments WHERE order_id = $1 AND store_id = $2)
		     OR EXISTS (SELECT 1 FROM invoice_orders io JOIN invoices i ON i.id = io.invoice_id
		                WHERE io.order_id = $1 AND i.store_id = $2)`, id, s.sid()).Scan(&has)
	return has, err
}

// NextDocNumber menghasilkan nomor dokumen berurutan per toko per bulan, mis. TRX/2026/10/0001.
// Atomik karena memakai upsert baris counter.
func (s *Store) NextDocNumber(ctx context.Context, prefix string, date time.Time) (string, error) {
	period := date.Format("2006/01")
	var n int
	err := s.db.QueryRow(ctx,
		`INSERT INTO doc_counters (store_id, prefix, period, last_no) VALUES ($3, $1, $2, 1)
		 ON CONFLICT (store_id, prefix, period) DO UPDATE SET last_no = doc_counters.last_no + 1
		 RETURNING last_no`, prefix, period, s.sid()).Scan(&n)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s/%04d", prefix, period, n), nil
}

// LineSubtotal = qty × harga, dibulatkan ke rupiah terdekat.
func LineSubtotal(qty float64, price int64) int64 {
	v := qty * float64(price)
	if v < 0 {
		return int64(v - 0.5)
	}
	return int64(v + 0.5)
}
