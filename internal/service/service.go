// Package service berisi aturan bisnis yang butuh transaksi database:
// pembuatan transaksi penjualan, pembayaran cicilan, invoice tagihan & pelunasan.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"rekapin/internal/money"
	"rekapin/internal/store"
)

// ValidationError berisi pesan yang aman ditampilkan ke admin.
type ValidationError struct{ Msg string }

func (e ValidationError) Error() string { return e.Msg }

func invalid(format string, a ...any) error { return ValidationError{Msg: fmt.Sprintf(format, a...)} }

// IsValidation true jika err berasal dari validasi input (bukan error sistem).
func IsValidation(err error) bool {
	var v ValidationError
	return errors.As(err, &v)
}

var validMethods = map[string]bool{"cash": true, "transfer": true, "lainnya": true}

type Service struct {
	st *store.Store
}

func New(st *store.Store) *Service { return &Service{st: st} }

// ForStore mengembalikan Service yang beroperasi pada data satu toko.
func (s *Service) ForStore(id int64) *Service { return &Service{st: s.st.ForStore(id)} }

// ---------- Transaksi penjualan ----------

func validateOrder(in *store.OrderInput) (int64, error) {
	if in.CustomerID == 0 {
		return 0, invalid("customer wajib dipilih")
	}
	if in.OrderDate.IsZero() {
		return 0, invalid("tanggal transaksi wajib diisi")
	}
	items := in.Items[:0]
	var total int64
	for _, it := range in.Items {
		it.ProductName = strings.TrimSpace(it.ProductName)
		if it.ProductName == "" {
			continue // baris kosong di form diabaikan
		}
		if it.Qty <= 0 {
			return 0, invalid("qty untuk %q harus lebih dari 0", it.ProductName)
		}
		if it.UnitPrice < 0 {
			return 0, invalid("harga untuk %q tidak boleh negatif", it.ProductName)
		}
		total += store.LineSubtotal(it.Qty, it.UnitPrice)
		items = append(items, it)
	}
	if len(items) == 0 {
		return 0, invalid("minimal satu barang harus diisi")
	}
	in.Items = items
	return total, nil
}

// CreateOrder menyimpan transaksi baru; downPayment > 0 dicatat sebagai pembayaran pertama.
func (s *Service) CreateOrder(ctx context.Context, in store.OrderInput, downPayment int64, dpMethod string, userID int64) (int64, error) {
	total, err := validateOrder(&in)
	if err != nil {
		return 0, err
	}
	if downPayment < 0 || downPayment > total {
		return 0, invalid("uang muka harus antara 0 dan total (Rp %s)", money.Format(total))
	}
	if downPayment > 0 && !validMethods[dpMethod] {
		return 0, invalid("metode pembayaran tidak valid")
	}
	var id int64
	err = s.st.WithTx(ctx, func(tx *store.Store) error {
		code, err := tx.NextDocNumber(ctx, "TRX", in.OrderDate)
		if err != nil {
			return err
		}
		if id, err = tx.InsertOrder(ctx, code, in, total, userID); err != nil {
			return err
		}
		if downPayment > 0 {
			_, err = tx.InsertPayment(ctx, store.PaymentInput{
				OrderID: id, Amount: downPayment, PaidAt: in.OrderDate, Method: dpMethod, Note: "Uang muka",
			}, userID)
		}
		return err
	})
	return id, err
}

// UpdateOrder hanya diizinkan selama order belum punya pembayaran / invoice,
// agar angka di log pembayaran & invoice tidak berubah diam-diam.
func (s *Service) UpdateOrder(ctx context.Context, id int64, in store.OrderInput) error {
	total, err := validateOrder(&in)
	if err != nil {
		return err
	}
	return s.st.WithTx(ctx, func(tx *store.Store) error {
		if _, err := tx.LockOrder(ctx, id); err != nil {
			return err
		}
		if has, err := tx.OrderHasHistory(ctx, id); err != nil {
			return err
		} else if has {
			return invalid("transaksi sudah memiliki pembayaran/invoice sehingga tidak bisa diubah")
		}
		return tx.ReplaceOrder(ctx, id, in, total)
	})
}

func (s *Service) DeleteOrder(ctx context.Context, id int64) error {
	return s.st.WithTx(ctx, func(tx *store.Store) error {
		if _, err := tx.LockOrder(ctx, id); err != nil {
			return err
		}
		if has, err := tx.OrderHasHistory(ctx, id); err != nil {
			return err
		} else if has {
			return invalid("transaksi sudah memiliki pembayaran/invoice sehingga tidak bisa dihapus")
		}
		return tx.DeleteOrder(ctx, id)
	})
}

// ---------- Pembayaran ----------

// AddPayment mencatat cicilan. Order dikunci (FOR UPDATE) agar dua input bersamaan
// tidak bisa membuat total bayar melebihi tagihan.
func (s *Service) AddPayment(ctx context.Context, in store.PaymentInput, userID int64) (int64, error) {
	if in.Amount <= 0 {
		return 0, invalid("nominal pembayaran harus lebih dari 0")
	}
	if !validMethods[in.Method] {
		return 0, invalid("metode pembayaran tidak valid")
	}
	if in.PaidAt.IsZero() {
		in.PaidAt = today()
	}
	var id int64
	err := s.st.WithTx(ctx, func(tx *store.Store) error {
		o, err := tx.LockOrder(ctx, in.OrderID)
		if err != nil {
			return err
		}
		if in.Amount > o.Remaining {
			return invalid("nominal melebihi sisa tagihan (Rp %s)", money.Format(o.Remaining))
		}
		id, err = tx.InsertPayment(ctx, in, userID)
		return err
	})
	return id, err
}

func (s *Service) VoidPayment(ctx context.Context, id, userID int64) error {
	return s.st.VoidPayment(ctx, id, userID)
}

// ---------- Invoice ----------

type InvoiceRequest struct {
	store.InvoiceInput
	OrderIDs []int64
	Method   string // khusus pelunasan
}

// CreateInvoice membuat invoice tagihan (snapshot sisa tiap order) atau
// invoice pelunasan (sekaligus mencatat pembayaran sebesar sisa tiap order).
func (s *Service) CreateInvoice(ctx context.Context, req InvoiceRequest, userID int64) (int64, error) {
	if req.CustomerID == 0 {
		return 0, invalid("customer wajib dipilih")
	}
	if len(req.OrderIDs) == 0 {
		return 0, invalid("pilih minimal satu transaksi")
	}
	if req.IssueDate.IsZero() {
		req.IssueDate = today()
	}
	prefix := "INV"
	switch req.Type {
	case "tagihan":
	case "pelunasan":
		prefix = "LNS"
		if !validMethods[req.Method] {
			return 0, invalid("metode pembayaran tidak valid")
		}
		req.DueDate = nil
	default:
		return 0, invalid("jenis invoice tidak valid")
	}

	var invID int64
	err := s.st.WithTx(ctx, func(tx *store.Store) error {
		number, err := tx.NextDocNumber(ctx, prefix, req.IssueDate)
		if err != nil {
			return err
		}
		if invID, err = tx.InsertInvoice(ctx, number, req.InvoiceInput, userID); err != nil {
			return err
		}
		for _, oid := range uniq(req.OrderIDs) {
			o, err := tx.LockOrder(ctx, oid)
			if err != nil {
				return err
			}
			if o.CustomerID != req.CustomerID {
				return invalid("transaksi %s bukan milik customer ini", o.Code)
			}
			if o.Remaining <= 0 {
				return invalid("transaksi %s sudah lunas", o.Code)
			}
			if err := tx.InsertInvoiceOrder(ctx, invID, oid, o.Remaining); err != nil {
				return err
			}
			if req.Type == "pelunasan" {
				if _, err := tx.InsertPayment(ctx, store.PaymentInput{
					OrderID: oid, Amount: o.Remaining, PaidAt: req.IssueDate,
					Method: req.Method, Note: "Pelunasan " + number, InvoiceID: &invID,
				}, userID); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return invID, err
}

func uniq(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	out := ids[:0:0]
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func today() time.Time {
	y, m, d := time.Now().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}
