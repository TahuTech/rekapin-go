package service_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rekapin/internal/db"
	"rekapin/internal/service"
	"rekapin/internal/store"
)

// Test integrasi: butuh database kosong khusus test, mis.
// TEST_DATABASE_URL=postgres://rekapin:rekapin@127.0.0.1:55432/rekapin_test?sslmode=disable
func setup(t *testing.T) (*store.Store, *service.Service, int64) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL tidak di-set")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dsn, 5)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	reset(t, pool)
	base := store.New(pool)
	sid := newTenant(t, base, "Toko Test")
	uid, err := base.CreateUser(ctx, store.UserInput{Username: "tester", Name: "Tester", Role: store.RoleAdmin, StoreID: &sid, Active: true}, "x")
	if err != nil {
		t.Fatal(err)
	}
	st := base.ForStore(sid)
	return st, service.New(base).ForStore(sid), uid
}

func newTenant(t *testing.T, st *store.Store, name string) int64 {
	t.Helper()
	id, err := st.CreateTenant(context.Background(), store.TenantInput{Name: name, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func reset(t *testing.T, pool *pgxpool.Pool) {
	_, err := pool.Exec(context.Background(),
		`TRUNCATE payments, invoice_orders, invoices, order_items, orders, products, customers, doc_counters, users, stores RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
}

var day = time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)

// newOrder membuat customer + order senilai total (qty 1).
func newOrder(t *testing.T, st *store.Store, svc *service.Service, uid int64, total int64) (custID, orderID int64) {
	t.Helper()
	ctx := context.Background()
	custID, err := st.CreateCustomer(ctx, store.CustomerInput{Name: "Budi"})
	if err != nil {
		t.Fatal(err)
	}
	orderID, err = svc.CreateOrder(ctx, store.OrderInput{
		CustomerID: custID, OrderDate: day,
		Items: []store.OrderItemInput{{ProductName: "Barang", Qty: 1, UnitPrice: total}},
	}, 0, "", uid)
	if err != nil {
		t.Fatal(err)
	}
	return custID, orderID
}

func TestCreateOrderTotalAndDownPayment(t *testing.T) {
	st, svc, uid := setup(t)
	ctx := context.Background()
	cid, _ := st.CreateCustomer(ctx, store.CustomerInput{Name: "Siti"})

	id, err := svc.CreateOrder(ctx, store.OrderInput{
		CustomerID: cid, OrderDate: day,
		Items: []store.OrderItemInput{
			{ProductName: "Beras", Qty: 2, UnitPrice: 75000},
			{ProductName: "Minyak", Qty: 1.5, UnitPrice: 36000},
			{ProductName: "  ", Qty: 1, UnitPrice: 1}, // baris kosong diabaikan
		},
	}, 50000, "cash", uid)
	if err != nil {
		t.Fatal(err)
	}
	o, _ := st.OrderByID(ctx, id)
	if o.Total != 204000 || o.Paid != 50000 || o.Remaining != 154000 || o.Status != "sebagian" {
		t.Fatalf("got total=%d paid=%d remaining=%d status=%s", o.Total, o.Paid, o.Remaining, o.Status)
	}
	if o.Code != "TRX/2026/10/0001" {
		t.Fatalf("kode = %s", o.Code)
	}

	// DP melebihi total ditolak.
	_, err = svc.CreateOrder(ctx, store.OrderInput{CustomerID: cid, OrderDate: day,
		Items: []store.OrderItemInput{{ProductName: "X", Qty: 1, UnitPrice: 1000}}}, 2000, "cash", uid)
	if !service.IsValidation(err) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestPaymentCannotExceedRemaining(t *testing.T) {
	st, svc, uid := setup(t)
	ctx := context.Background()
	_, oid := newOrder(t, st, svc, uid, 100000)

	if _, err := svc.AddPayment(ctx, store.PaymentInput{OrderID: oid, Amount: 60000, Method: "cash", PaidAt: day}, uid); err != nil {
		t.Fatal(err)
	}
	_, err := svc.AddPayment(ctx, store.PaymentInput{OrderID: oid, Amount: 50000, Method: "cash", PaidAt: day}, uid)
	if !service.IsValidation(err) {
		t.Fatalf("expected overpayment rejected, got %v", err)
	}
	o, _ := st.OrderByID(ctx, oid)
	if o.Remaining != 40000 {
		t.Fatalf("remaining = %d", o.Remaining)
	}
}

// Pembayaran bersamaan tidak boleh melampaui total berkat SELECT ... FOR UPDATE.
func TestConcurrentPayments(t *testing.T) {
	st, svc, uid := setup(t)
	ctx := context.Background()
	_, oid := newOrder(t, st, svc, uid, 100000)

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.AddPayment(ctx, store.PaymentInput{OrderID: oid, Amount: 30000, Method: "cash", PaidAt: day}, uid)
		}()
	}
	wg.Wait()
	o, _ := st.OrderByID(ctx, oid)
	if o.Paid != 90000 {
		t.Fatalf("paid = %d, want 90000 (3 dari 5 pembayaran lolos)", o.Paid)
	}
}

func TestInvoiceTagihanThenPelunasan(t *testing.T) {
	st, svc, uid := setup(t)
	ctx := context.Background()
	cid, oid := newOrder(t, st, svc, uid, 100000)
	if _, err := svc.AddPayment(ctx, store.PaymentInput{OrderID: oid, Amount: 30000, Method: "cash", PaidAt: day}, uid); err != nil {
		t.Fatal(err)
	}

	invID, err := svc.CreateInvoice(ctx, service.InvoiceRequest{
		InvoiceInput: store.InvoiceInput{Type: "tagihan", CustomerID: cid, IssueDate: day},
		OrderIDs:     []int64{oid, oid}, // duplikat diabaikan
	}, uid)
	if err != nil {
		t.Fatal(err)
	}
	inv, _ := st.InvoiceByID(ctx, invID)
	if inv.Number != "INV/2026/10/0001" || inv.Total != 70000 {
		t.Fatalf("invoice %s total %d", inv.Number, inv.Total)
	}

	lnsID, err := svc.CreateInvoice(ctx, service.InvoiceRequest{
		InvoiceInput: store.InvoiceInput{Type: "pelunasan", CustomerID: cid, IssueDate: day},
		OrderIDs:     []int64{oid}, Method: "transfer",
	}, uid)
	if err != nil {
		t.Fatal(err)
	}
	o, _ := st.OrderByID(ctx, oid)
	if o.Status != "lunas" || o.Remaining != 0 {
		t.Fatalf("status=%s remaining=%d", o.Status, o.Remaining)
	}
	pays, _ := st.InvoicePayments(ctx, lnsID)
	if len(pays) != 1 || pays[0].Amount != 70000 {
		t.Fatalf("pelunasan payments = %+v", pays)
	}

	// Order lunas tidak bisa ditagih / dilunasi lagi, dan tidak bisa diedit.
	_, err = svc.CreateInvoice(ctx, service.InvoiceRequest{
		InvoiceInput: store.InvoiceInput{Type: "tagihan", CustomerID: cid}, OrderIDs: []int64{oid},
	}, uid)
	if !service.IsValidation(err) {
		t.Fatalf("expected validation error, got %v", err)
	}
	if err := svc.DeleteOrder(ctx, oid); !service.IsValidation(err) {
		t.Fatalf("expected delete blocked, got %v", err)
	}
}

func TestInvoiceRejectsOtherCustomersOrder(t *testing.T) {
	st, svc, uid := setup(t)
	ctx := context.Background()
	_, oid := newOrder(t, st, svc, uid, 50000)
	other, _ := st.CreateCustomer(ctx, store.CustomerInput{Name: "Lain"})

	_, err := svc.CreateInvoice(ctx, service.InvoiceRequest{
		InvoiceInput: store.InvoiceInput{Type: "tagihan", CustomerID: other}, OrderIDs: []int64{oid},
	}, uid)
	if !service.IsValidation(err) {
		t.Fatalf("expected validation error, got %v", err)
	}
	// Transaksi gagal di-rollback: tidak ada invoice yatim & nomor tetap bisa dipakai ulang.
	list, total, _ := st.ListInvoices(ctx, store.InvoiceFilter{}, store.Page{})
	if total != 0 || len(list) != 0 {
		t.Fatalf("invoice tersisa setelah rollback: %d", total)
	}
}

func TestVoidRestoresRemaining(t *testing.T) {
	st, svc, uid := setup(t)
	ctx := context.Background()
	_, oid := newOrder(t, st, svc, uid, 80000)
	pid, err := svc.AddPayment(ctx, store.PaymentInput{OrderID: oid, Amount: 80000, Method: "cash", PaidAt: day}, uid)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.VoidPayment(ctx, pid, uid); err != nil {
		t.Fatal(err)
	}
	o, _ := st.OrderByID(ctx, oid)
	if o.Remaining != 80000 || o.Status != "belum" {
		t.Fatalf("remaining=%d status=%s", o.Remaining, o.Status)
	}
	if err := svc.VoidPayment(ctx, pid, uid); err == nil {
		t.Fatal("void kedua seharusnya gagal")
	}
}

// Data toko lain tidak boleh terbaca maupun termodifikasi lewat Store/Service toko berbeda.
func TestStoreIsolation(t *testing.T) {
	stA, svcA, uid := setup(t)
	ctx := context.Background()
	cidA, oidA := newOrder(t, stA, svcA, uid, 100000)

	stB := stA.ForStore(newTenant(t, stA, "Toko B"))
	svcB := service.New(stA).ForStore(stB.StoreID())

	// Baca lintas toko -> tidak ditemukan / kosong.
	if _, err := stB.OrderByID(ctx, oidA); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("OrderByID lintas toko: %v", err)
	}
	if _, err := stB.CustomerByID(ctx, cidA); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("CustomerByID lintas toko: %v", err)
	}
	if list, total, err := stB.ListOrders(ctx, store.OrderFilter{}, store.Page{}); err != nil || total != 0 || len(list) != 0 {
		t.Fatalf("ListOrders toko B: total=%d err=%v", total, err)
	}
	if d, err := stB.Dashboard(ctx, day, day); err != nil || d.Receivable != 0 || d.CustomerCount != 0 {
		t.Fatalf("dashboard toko B bocor: %+v err=%v", d, err)
	}

	// Tulis lintas toko -> ditolak.
	if _, err := svcB.AddPayment(ctx, store.PaymentInput{OrderID: oidA, Amount: 1000, Method: "cash", PaidAt: day}, uid); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("AddPayment lintas toko: %v", err)
	}
	if err := svcB.DeleteOrder(ctx, oidA); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteOrder lintas toko: %v", err)
	}
	if err := stB.UpdateCustomer(ctx, cidA, store.CustomerInput{Name: "Hack"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("UpdateCustomer lintas toko: %v", err)
	}
	// FK komposit menolak order toko B yang menunjuk customer toko A.
	_, err := svcB.CreateOrder(ctx, store.OrderInput{CustomerID: cidA, OrderDate: day,
		Items: []store.OrderItemInput{{ProductName: "X", Qty: 1, UnitPrice: 1000}}}, 0, "", uid)
	if err == nil {
		t.Fatal("order ke customer toko lain seharusnya gagal")
	}

	// Nomor dokumen dihitung per toko.
	_, oidB := newOrder(t, stB, svcB, uid, 5000)
	if o, _ := stB.OrderByID(ctx, oidB); o.Code != "TRX/2026/10/0001" {
		t.Fatalf("kode toko B = %s, want TRX/2026/10/0001", o.Code)
	}
	if o, _ := stA.OrderByID(ctx, oidA); o.Remaining != 100000 {
		t.Fatalf("order toko A berubah: remaining=%d", o.Remaining)
	}
}

func TestUnscopedStorePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("query tanpa ForStore seharusnya panic")
		}
	}()
	_, _ = store.New(nil).CustomerOptions(context.Background())
}
