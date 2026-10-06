# Rekapin

Sistem rekap penjualan & piutang **khusus admin**: customer, barang yang dibeli, total beli,
log pembayaran (cicilan), sisa pembayaran, invoice tagihan, pelunasan (kwitansi), serta rekap
per customer dan per periode (+ export CSV).

Stack: Go (stdlib `net/http`) · HTMX · Tailwind v4 · PostgreSQL. Satu binary statis (template &
asset ter-embed), RAM aplikasi ~25–30 MB — dirancang untuk VPS 1 GB / 1 core.

## Fitur

| Modul | Isi |
|---|---|
| Dashboard | Total piutang, penjualan & penerimaan bulan ini, customer dengan sisa terbesar, pembayaran terakhir |
| Customer | CRUD, pencarian live, ringkasan total beli / terbayar / sisa, tab transaksi, log pembayaran, invoice |
| Barang | Master produk (harga default, satuan, aktif/nonaktif) |
| Penjualan | Multi item (pilih dari master atau manual, harga bisa diubah), uang muka, filter status/tanggal |
| Pembayaran | Cicilan per transaksi, validasi tidak boleh melebihi sisa (row lock), void dengan jejak audit |
| Invoice | **Tagihan** (snapshot sisa beberapa transaksi) & **Pelunasan** (otomatis mencatat pembayaran + kwitansi LUNAS); cetak / Save as PDF |
| Rekap | Per customer & per periode (harian/bulanan), barang terlaris, export CSV (`;`, siap Excel) |

Nomor dokumen otomatis per bulan: `TRX/2026/10/0001`, `INV/…`, `LNS/…`.

## Development

### Opsi A — Docker (disarankan, host cukup punya Docker)

```bash
make dev          # Postgres + app dengan hot reload → http://localhost:8080
make admin        # (terminal lain) buat akun master dev: admin / admin12345
```

Setiap perubahan `.go`, `.html`, `.css`, `.js`, atau migrasi `.sql` otomatis memicu build Tailwind →
`go build` → restart (±2 detik). Refresh browser untuk melihat hasilnya.

| Perintah | Fungsi |
|---|---|
| `make dev` / `make dev-down` | Jalankan / hentikan lingkungan dev |
| `make dev-reset` | Hentikan + **hapus data** DB dev |
| `make logs` | Ikuti log app |
| `make psql` | Masuk psql database dev |
| `make test-docker` | `go vet` + seluruh test (integration test memakai DB `rekapin_test`) |
| `make build-docker` | Build binary produksi `bin/rekapin` (linux/amd64) tanpa Go di host |

Port bisa diubah bila bentrok: `DB_PORT=5434 APP_PORT=8081 make dev`
(default DB `127.0.0.1:5433`, app `127.0.0.1:8080`).

Bila perubahan file tidak terdeteksi (Docker Desktop macOS/Windows), set `poll = true` di `.air.toml`.

### Multi-toko & akun master

- **Master** login ke `/master`: membuat toko, membuat/menonaktifkan admin per toko, dan
  "Buka toko" untuk melihat data toko mana pun (banner kuning menandai mode ini).
- **Admin** hanya melihat data tokonya sendiri; nama & alamat toko (sidebar, kop invoice)
  diatur di menu master, bukan lagi dari `COMPANY_NAME`/`COMPANY_INFO`.
- CLI: `rekapin user create -role master <username> <nama>`,
  `rekapin store create <nama>`, `rekapin user create -store <id> <username> <nama>`.

### Opsi B — Go native (Go ≥ 1.26)

```bash
make db-up                      # Postgres saja (container)
export DATABASE_URL='postgres://rekapin:rekapin@127.0.0.1:5433/rekapin?sslmode=disable'
make css                        # unduh Tailwind standalone CLI & build CSS
go run ./cmd/rekapin user create -role master admin "Administrator"
make run                        # http://127.0.0.1:8080 (jalankan `make css-watch` di terminal lain)

TEST_DATABASE_URL='postgres://rekapin:rekapin@127.0.0.1:5433/rekapin_test?sslmode=disable' make test
```

## Deploy ke VPS (Ubuntu/Debian, 1 GB RAM)

```bash
# 1. Swap 1 GB (penting di RAM 1 GB)
sudo fallocate -l 1G /swapfile && sudo chmod 600 /swapfile && sudo mkswap /swapfile && sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
echo 'vm.swappiness=10' | sudo tee /etc/sysctl.d/99-swap.conf && sudo sysctl --system

# 2. PostgreSQL + tuning
sudo apt install -y postgresql
sudo cp deploy/postgresql.conf.snippet /etc/postgresql/*/main/conf.d/rekapin.conf
sudo systemctl restart postgresql
sudo -u postgres psql -c "CREATE USER rekapin WITH PASSWORD 'GANTI_PASSWORD';"
sudo -u postgres psql -c "CREATE DATABASE rekapin OWNER rekapin;"

# 3. Aplikasi (build di laptop: make build-docker atau make build -> bin/rekapin, lalu scp)
sudo useradd --system --home /opt/rekapin --shell /usr/sbin/nologin rekapin
sudo mkdir -p /opt/rekapin && sudo cp bin/rekapin /opt/rekapin/
sudo cp .env.example /opt/rekapin/.env && sudo nano /opt/rekapin/.env
sudo chown -R root:rekapin /opt/rekapin && sudo chmod 640 /opt/rekapin/.env
sudo cp deploy/rekapin.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now rekapin
cd /opt/rekapin && sudo -u rekapin env $(sudo cat .env | xargs) ./rekapin user create -role master admin "Administrator"

# 4. Caddy (HTTPS otomatis)
sudo apt install -y caddy
sudo cp deploy/Caddyfile /etc/caddy/Caddyfile   # ganti domain
sudo systemctl reload caddy

# 5. Backup harian
sudo cp deploy/backup.sh /opt/rekapin/ && sudo -u postgres crontab -l 2>/dev/null | { cat; echo '15 2 * * * /opt/rekapin/backup.sh'; } | sudo -u postgres crontab -
```

Update versi: `make build-docker` (atau `make build`), salin binary baru ke `/opt/rekapin/`, `sudo systemctl restart rekapin`
(migrasi skema berjalan otomatis saat start).

## Struktur

```
cmd/rekapin/          entry point: serve | migrate | user create | store create
internal/config       konfigurasi dari env
internal/db           pgxpool + migrasi goose (SQL ter-embed)
internal/store        query per domain (pgx)
internal/service      aturan bisnis bertransaksi: order, cicilan, invoice/pelunasan
internal/web          handler HTTP, middleware (auth, CSRF, log), render template
web/templates         layout, partial, halaman (html/template)
web/static            app.css (hasil build), htmx.min.js, app.js
deploy/               systemd, Caddyfile, tuning PostgreSQL, backup (+ dev/initdb.sql)
compose.yaml          lingkungan dev (db + app hot reload); Dockerfile.dev, .air.toml
```

## Catatan desain

- Nominal disimpan `BIGINT` rupiah (tanpa desimal); qty `NUMERIC(12,2)`.
- Saldo dihitung lewat view `order_balances` (total − pembayaran non-void), jadi tidak ada kolom saldo yang bisa tidak sinkron.
- Transaksi yang sudah punya pembayaran/invoice dikunci (tidak bisa diedit/hapus); koreksi dilakukan dengan void pembayaran.
- Proteksi CSRF memakai `http.CrossOriginProtection` (Go 1.25+), session di PostgreSQL (`scs`), password bcrypt.
