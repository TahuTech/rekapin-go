-- +goose Up
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Skema bawaan scs/pgxstore
CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BYTEA NOT NULL,
    expiry TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);

CREATE TABLE customers (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    phone       TEXT NOT NULL DEFAULT '',
    address     TEXT NOT NULL DEFAULT '',
    notes       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at TIMESTAMPTZ
);
CREATE INDEX customers_name_idx ON customers (lower(name));

CREATE TABLE products (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    unit       TEXT NOT NULL DEFAULT 'pcs',
    price      BIGINT NOT NULL DEFAULT 0 CHECK (price >= 0),
    active     BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id          BIGSERIAL PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,
    customer_id BIGINT NOT NULL REFERENCES customers(id),
    order_date  DATE NOT NULL,
    notes       TEXT NOT NULL DEFAULT '',
    total       BIGINT NOT NULL CHECK (total >= 0),
    created_by  BIGINT REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX orders_customer_date_idx ON orders (customer_id, order_date);
CREATE INDEX orders_date_idx ON orders (order_date);

CREATE TABLE order_items (
    id           BIGSERIAL PRIMARY KEY,
    order_id     BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id   BIGINT REFERENCES products(id) ON DELETE SET NULL,
    product_name TEXT NOT NULL, -- snapshot nama saat transaksi
    unit         TEXT NOT NULL DEFAULT '',
    qty          NUMERIC(12,2) NOT NULL CHECK (qty > 0),
    unit_price   BIGINT NOT NULL CHECK (unit_price >= 0),
    subtotal     BIGINT NOT NULL CHECK (subtotal >= 0)
);
CREATE INDEX order_items_order_idx ON order_items (order_id);
CREATE INDEX order_items_product_idx ON order_items (product_id);

CREATE TABLE invoices (
    id          BIGSERIAL PRIMARY KEY,
    number      TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL CHECK (type IN ('tagihan', 'pelunasan')),
    customer_id BIGINT NOT NULL REFERENCES customers(id),
    issue_date  DATE NOT NULL,
    due_date    DATE,
    notes       TEXT NOT NULL DEFAULT '',
    created_by  BIGINT REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX invoices_customer_idx ON invoices (customer_id, issue_date);

CREATE TABLE invoice_orders (
    invoice_id BIGINT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    order_id   BIGINT NOT NULL REFERENCES orders(id),
    amount_due BIGINT NOT NULL, -- snapshot sisa tagihan saat invoice dibuat
    PRIMARY KEY (invoice_id, order_id)
);

CREATE TABLE payments (
    id         BIGSERIAL PRIMARY KEY,
    order_id   BIGINT NOT NULL REFERENCES orders(id),
    amount     BIGINT NOT NULL CHECK (amount > 0),
    paid_at    DATE NOT NULL,
    method     TEXT NOT NULL CHECK (method IN ('cash', 'transfer', 'lainnya')),
    note       TEXT NOT NULL DEFAULT '',
    invoice_id BIGINT REFERENCES invoices(id),
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Pembayaran tidak dihapus fisik agar log tetap utuh; cukup di-void.
    voided_at  TIMESTAMPTZ,
    voided_by  BIGINT REFERENCES users(id)
);
CREATE INDEX payments_order_idx ON payments (order_id) WHERE voided_at IS NULL;
CREATE INDEX payments_paid_at_idx ON payments (paid_at);

-- Penomoran dokumen atomik per prefix & periode (YYYYMM).
CREATE TABLE doc_counters (
    prefix  TEXT NOT NULL,
    period  TEXT NOT NULL,
    last_no INT  NOT NULL,
    PRIMARY KEY (prefix, period)
);

-- Saldo per transaksi: total, terbayar, sisa, status.
CREATE VIEW order_balances AS
SELECT o.id AS order_id,
       o.customer_id,
       o.total,
       COALESCE(p.paid, 0)::BIGINT           AS paid,
       (o.total - COALESCE(p.paid, 0))::BIGINT AS remaining,
       CASE
           WHEN o.total - COALESCE(p.paid, 0) <= 0 THEN 'lunas'
           WHEN COALESCE(p.paid, 0) > 0 THEN 'sebagian'
           ELSE 'belum'
       END AS status
FROM orders o
LEFT JOIN (
    SELECT order_id, SUM(amount) AS paid
    FROM payments
    WHERE voided_at IS NULL
    GROUP BY order_id
) p ON p.order_id = o.id;

-- +goose Down
DROP VIEW order_balances;
DROP TABLE doc_counters, payments, invoice_orders, invoices, order_items, orders, products, customers, sessions, users;
