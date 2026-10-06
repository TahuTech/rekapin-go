package store

import (
	"context"
	"time"
)

type Dashboard struct {
	Receivable       int64 `db:"receivable"`
	OutstandingCount int64 `db:"outstanding_count"`
	SalesMonth       int64 `db:"sales_month"`
	ReceiptsMonth    int64 `db:"receipts_month"`
	CustomerCount    int64 `db:"customer_count"`
}

func (s *Store) Dashboard(ctx context.Context, monthStart, monthEnd time.Time) (Dashboard, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
		  (SELECT COALESCE(SUM(remaining), 0) FROM order_balances WHERE remaining > 0)::BIGINT AS receivable,
		  (SELECT COUNT(*) FROM order_balances WHERE remaining > 0)                          AS outstanding_count,
		  (SELECT COALESCE(SUM(total), 0) FROM orders WHERE order_date BETWEEN $1 AND $2)::BIGINT AS sales_month,
		  (SELECT COALESCE(SUM(amount), 0) FROM payments
		    WHERE voided_at IS NULL AND paid_at BETWEEN $1 AND $2)::BIGINT                   AS receipts_month,
		  (SELECT COUNT(*) FROM customers WHERE archived_at IS NULL)                         AS customer_count`,
		monthStart, monthEnd)
	return collectOne[Dashboard](rows, err)
}

// TopDebtors = customer dengan sisa tagihan terbesar.
func (s *Store) TopDebtors(ctx context.Context, limit int) ([]CustomerSummary, error) {
	rows, err := s.db.Query(ctx, customerSummarySQL+`
		GROUP BY c.id HAVING COALESCE(SUM(b.remaining), 0) > 0
		ORDER BY remaining DESC LIMIT $1`, limit)
	return collect[CustomerSummary](rows, err)
}

type CustomerReportRow struct {
	CustomerID   int64  `db:"customer_id"`
	CustomerName string `db:"customer_name"`
	OrderCount   int64  `db:"order_count"`
	TotalBuy     int64  `db:"total_buy"`
	TotalPaid    int64  `db:"total_paid"`
	Remaining    int64  `db:"remaining"`
	Receipts     int64  `db:"receipts"` // uang masuk pada periode (berdasar tanggal bayar)
}

// CustomerReport merekap per customer: transaksi bertanggal dalam periode beserta
// pembayaran & sisanya, ditambah penerimaan kas dalam periode.
func (s *Store) CustomerReport(ctx context.Context, r DateRange, customerID int64) ([]CustomerReportRow, error) {
	rows, err := s.db.Query(ctx, `
		WITH ord AS (
		  SELECT o.customer_id, COUNT(*) AS order_count, SUM(b.total) AS total_buy,
		         SUM(b.paid) AS total_paid, SUM(b.remaining) AS remaining
		  FROM orders o JOIN order_balances b ON b.order_id = o.id
		  WHERE ($1::date IS NULL OR o.order_date >= $1) AND ($2::date IS NULL OR o.order_date <= $2)
		  GROUP BY o.customer_id
		), rcv AS (
		  SELECT o.customer_id, SUM(p.amount) AS receipts
		  FROM payments p JOIN orders o ON o.id = p.order_id
		  WHERE p.voided_at IS NULL
		    AND ($1::date IS NULL OR p.paid_at >= $1) AND ($2::date IS NULL OR p.paid_at <= $2)
		  GROUP BY o.customer_id
		)
		SELECT c.id AS customer_id, c.name AS customer_name,
		       COALESCE(ord.order_count, 0)        AS order_count,
		       COALESCE(ord.total_buy, 0)::BIGINT  AS total_buy,
		       COALESCE(ord.total_paid, 0)::BIGINT AS total_paid,
		       COALESCE(ord.remaining, 0)::BIGINT  AS remaining,
		       COALESCE(rcv.receipts, 0)::BIGINT   AS receipts
		FROM customers c
		LEFT JOIN ord ON ord.customer_id = c.id
		LEFT JOIN rcv ON rcv.customer_id = c.id
		WHERE (ord.customer_id IS NOT NULL OR rcv.customer_id IS NOT NULL)
		  AND ($3::bigint = 0 OR c.id = $3)
		ORDER BY total_buy DESC, c.name`, r.From, r.To, customerID)
	return collect[CustomerReportRow](rows, err)
}

type PeriodRow struct {
	Bucket     time.Time `db:"bucket"`
	OrderCount int64     `db:"order_count"`
	Sales      int64     `db:"sales"`
	Receipts   int64     `db:"receipts"`
}

// PeriodReport merekap penjualan vs penerimaan per hari/bulan. unit: "day" | "month".
// Periode wajib dibatasi (from & to) agar generate_series tidak membengkak.
func (s *Store) PeriodReport(ctx context.Context, from, to time.Time, unit string) ([]PeriodRow, error) {
	if unit != "month" {
		unit = "day"
	}
	rows, err := s.db.Query(ctx, `
		WITH buckets AS (
		  SELECT generate_series(date_trunc($3, $1::date), date_trunc($3, $2::date), ('1 ' || $3)::interval)::date AS bucket
		), ord AS (
		  SELECT date_trunc($3, order_date)::date AS bucket, COUNT(*) AS order_count, SUM(total) AS sales
		  FROM orders WHERE order_date BETWEEN $1 AND $2 GROUP BY 1
		), rcv AS (
		  SELECT date_trunc($3, paid_at)::date AS bucket, SUM(amount) AS receipts
		  FROM payments WHERE voided_at IS NULL AND paid_at BETWEEN $1 AND $2 GROUP BY 1
		)
		SELECT b.bucket, COALESCE(ord.order_count, 0) AS order_count,
		       COALESCE(ord.sales, 0)::BIGINT AS sales, COALESCE(rcv.receipts, 0)::BIGINT AS receipts
		FROM buckets b
		LEFT JOIN ord ON ord.bucket = b.bucket
		LEFT JOIN rcv ON rcv.bucket = b.bucket
		ORDER BY b.bucket`, from, to, unit)
	return collect[PeriodRow](rows, err)
}

type ProductReportRow struct {
	ProductName string  `db:"product_name"`
	Unit        string  `db:"unit"`
	Qty         float64 `db:"qty"`
	Revenue     int64   `db:"revenue"`
}

func (s *Store) TopProducts(ctx context.Context, r DateRange, customerID int64, limit int) ([]ProductReportRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT i.product_name, i.unit, SUM(i.qty)::float8 AS qty, SUM(i.subtotal)::BIGINT AS revenue
		FROM order_items i JOIN orders o ON o.id = i.order_id
		WHERE ($1::date IS NULL OR o.order_date >= $1) AND ($2::date IS NULL OR o.order_date <= $2)
		  AND ($3::bigint = 0 OR o.customer_id = $3)
		GROUP BY i.product_name, i.unit
		ORDER BY revenue DESC LIMIT $4`, r.From, r.To, customerID, limit)
	return collect[ProductReportRow](rows, err)
}
