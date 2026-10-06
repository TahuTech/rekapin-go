-- +goose Up
-- Multi-toko: setiap data operasional milik satu toko; user "master" mengelola semua toko.
CREATE TABLE stores (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    info       TEXT NOT NULL DEFAULT '',
    active     BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Toko default untuk menampung data & user yang sudah ada sebelum migrasi ini.
INSERT INTO stores (name) VALUES ('Toko Utama');

ALTER TABLE users
    ADD COLUMN role     TEXT NOT NULL DEFAULT 'admin' CHECK (role IN ('master', 'admin')),
    ADD COLUMN store_id BIGINT REFERENCES stores(id),
    ADD COLUMN active   BOOLEAN NOT NULL DEFAULT true;
UPDATE users SET store_id = (SELECT min(id) FROM stores);
-- Master tidak terikat toko; admin wajib punya toko.
ALTER TABLE users ADD CONSTRAINT users_role_store_chk CHECK ((role = 'master') = (store_id IS NULL));
CREATE INDEX users_store_idx ON users (store_id);

-- Tambah store_id ke tabel domain, backfill ke toko default.
ALTER TABLE customers ADD COLUMN store_id BIGINT REFERENCES stores(id);
ALTER TABLE products  ADD COLUMN store_id BIGINT REFERENCES stores(id);
ALTER TABLE orders    ADD COLUMN store_id BIGINT REFERENCES stores(id);
ALTER TABLE invoices  ADD COLUMN store_id BIGINT REFERENCES stores(id);
ALTER TABLE payments  ADD COLUMN store_id BIGINT REFERENCES stores(id);
UPDATE customers SET store_id = (SELECT min(id) FROM stores);
UPDATE products  SET store_id = (SELECT min(id) FROM stores);
UPDATE orders    SET store_id = (SELECT min(id) FROM stores);
UPDATE invoices  SET store_id = (SELECT min(id) FROM stores);
UPDATE payments  SET store_id = (SELECT min(id) FROM stores);
ALTER TABLE customers ALTER COLUMN store_id SET NOT NULL;
ALTER TABLE products  ALTER COLUMN store_id SET NOT NULL;
ALTER TABLE orders    ALTER COLUMN store_id SET NOT NULL;
ALTER TABLE invoices  ALTER COLUMN store_id SET NOT NULL;
ALTER TABLE payments  ALTER COLUMN store_id SET NOT NULL;

-- (store_id, id) unik agar bisa dirujuk FK komposit: DB menolak relasi lintas toko
-- (mis. order toko A ke customer toko B) walaupun ada bug di query aplikasi.
ALTER TABLE customers ADD CONSTRAINT customers_store_id_key UNIQUE (store_id, id);
ALTER TABLE products  ADD CONSTRAINT products_store_id_key  UNIQUE (store_id, id);
ALTER TABLE orders    ADD CONSTRAINT orders_store_id_key    UNIQUE (store_id, id);
ALTER TABLE invoices  ADD CONSTRAINT invoices_store_id_key  UNIQUE (store_id, id);

ALTER TABLE orders   ADD CONSTRAINT orders_store_customer_fk
    FOREIGN KEY (store_id, customer_id) REFERENCES customers (store_id, id);
ALTER TABLE invoices ADD CONSTRAINT invoices_store_customer_fk
    FOREIGN KEY (store_id, customer_id) REFERENCES customers (store_id, id);
ALTER TABLE payments ADD CONSTRAINT payments_store_order_fk
    FOREIGN KEY (store_id, order_id) REFERENCES orders (store_id, id);
ALTER TABLE payments ADD CONSTRAINT payments_store_invoice_fk
    FOREIGN KEY (store_id, invoice_id) REFERENCES invoices (store_id, id);

-- Nomor dokumen unik per toko, bukan global.
ALTER TABLE orders   DROP CONSTRAINT orders_code_key,     ADD CONSTRAINT orders_store_code_key     UNIQUE (store_id, code);
ALTER TABLE invoices DROP CONSTRAINT invoices_number_key, ADD CONSTRAINT invoices_store_number_key UNIQUE (store_id, number);

CREATE INDEX customers_store_name_idx ON customers (store_id, lower(name));
CREATE INDEX products_store_idx       ON products (store_id);
CREATE INDEX orders_store_date_idx    ON orders (store_id, order_date);
CREATE INDEX invoices_store_date_idx  ON invoices (store_id, issue_date);
CREATE INDEX payments_store_paid_idx  ON payments (store_id, paid_at);

-- Counter penomoran per toko.
ALTER TABLE doc_counters ADD COLUMN store_id BIGINT REFERENCES stores(id);
UPDATE doc_counters SET store_id = (SELECT min(id) FROM stores);
ALTER TABLE doc_counters ALTER COLUMN store_id SET NOT NULL;
ALTER TABLE doc_counters DROP CONSTRAINT doc_counters_pkey, ADD PRIMARY KEY (store_id, prefix, period);

-- View saldo kini membawa store_id agar bisa difilter per toko.
DROP VIEW order_balances;
CREATE VIEW order_balances AS
SELECT o.id AS order_id,
       o.store_id,
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

-- Down hanya aman bila data berasal dari satu toko (nomor dokumen bisa bentrok).
ALTER TABLE doc_counters DROP CONSTRAINT doc_counters_pkey;
ALTER TABLE doc_counters DROP COLUMN store_id;
ALTER TABLE doc_counters ADD PRIMARY KEY (prefix, period);

ALTER TABLE orders   DROP CONSTRAINT orders_store_code_key,     ADD CONSTRAINT orders_code_key     UNIQUE (code);
ALTER TABLE invoices DROP CONSTRAINT invoices_store_number_key, ADD CONSTRAINT invoices_number_key UNIQUE (number);

-- DROP COLUMN ikut menghapus FK, unique, dan index yang memakai kolom tersebut.
ALTER TABLE payments  DROP COLUMN store_id;
ALTER TABLE invoices  DROP COLUMN store_id;
ALTER TABLE orders    DROP COLUMN store_id;
ALTER TABLE products  DROP COLUMN store_id;
ALTER TABLE customers DROP COLUMN store_id;

DELETE FROM users WHERE role = 'master';
ALTER TABLE users DROP COLUMN active, DROP COLUMN role, DROP COLUMN store_id;
DROP TABLE stores;
