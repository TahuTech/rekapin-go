package store

import "time"

const (
	RoleMaster = "master" // mengelola semua toko & akun admin
	RoleAdmin  = "admin"  // admin satu toko
)

type User struct {
	ID           int64     `db:"id"`
	Username     string    `db:"username"`
	Name         string    `db:"name"`
	PasswordHash string    `db:"password_hash"`
	Role         string    `db:"role"`
	StoreID      *int64    `db:"store_id"` // nil untuk master
	StoreName    *string   `db:"store_name"`
	Active       bool      `db:"active"`
	CreatedAt    time.Time `db:"created_at"`
}

func (u *User) IsMaster() bool { return u != nil && u.Role == RoleMaster }

// Tenant = satu toko pengguna aplikasi (tabel stores).
type Tenant struct {
	ID         int64     `db:"id"`
	Name       string    `db:"name"`
	Info       string    `db:"info"`
	Active     bool      `db:"active"`
	CreatedAt  time.Time `db:"created_at"`
	AdminCount int64     `db:"admin_count"`
}

type Customer struct {
	ID         int64      `db:"id"`
	StoreID    int64      `db:"store_id"`
	Name       string     `db:"name"`
	Phone      string     `db:"phone"`
	Address    string     `db:"address"`
	Notes      string     `db:"notes"`
	CreatedAt  time.Time  `db:"created_at"`
	ArchivedAt *time.Time `db:"archived_at"`
}

// CustomerSummary = customer + agregat total beli, bayar, sisa.
type CustomerSummary struct {
	Customer
	OrderCount int64 `db:"order_count"`
	TotalBuy   int64 `db:"total_buy"`
	TotalPaid  int64 `db:"total_paid"`
	Remaining  int64 `db:"remaining"`
}

type Product struct {
	ID        int64     `db:"id"`
	StoreID   int64     `db:"store_id"`
	Name      string    `db:"name"`
	Unit      string    `db:"unit"`
	Price     int64     `db:"price"`
	Active    bool      `db:"active"`
	CreatedAt time.Time `db:"created_at"`
}

type Order struct {
	ID           int64     `db:"id"`
	Code         string    `db:"code"`
	CustomerID   int64     `db:"customer_id"`
	CustomerName string    `db:"customer_name"`
	OrderDate    time.Time `db:"order_date"`
	Notes        string    `db:"notes"`
	Total        int64     `db:"total"`
	Paid         int64     `db:"paid"`
	Remaining    int64     `db:"remaining"`
	Status       string    `db:"status"` // belum | sebagian | lunas
	ItemsSummary string    `db:"items_summary"`
	CreatedAt    time.Time `db:"created_at"`
}

type OrderItem struct {
	ID          int64   `db:"id"`
	OrderID     int64   `db:"order_id"`
	ProductID   *int64  `db:"product_id"`
	ProductName string  `db:"product_name"`
	Unit        string  `db:"unit"`
	Qty         float64 `db:"qty"`
	UnitPrice   int64   `db:"unit_price"`
	Subtotal    int64   `db:"subtotal"`
}

type Payment struct {
	ID            int64      `db:"id"`
	OrderID       int64      `db:"order_id"`
	OrderCode     string     `db:"order_code"`
	CustomerID    int64      `db:"customer_id"`
	CustomerName  string     `db:"customer_name"`
	Amount        int64      `db:"amount"`
	PaidAt        time.Time  `db:"paid_at"`
	Method        string     `db:"method"`
	Note          string     `db:"note"`
	InvoiceID     *int64     `db:"invoice_id"`
	InvoiceNumber *string    `db:"invoice_number"`
	CreatedByName *string    `db:"created_by_name"`
	CreatedAt     time.Time  `db:"created_at"`
	VoidedAt      *time.Time `db:"voided_at"`
}

type Invoice struct {
	ID              int64      `db:"id"`
	Number          string     `db:"number"`
	Type            string     `db:"type"` // tagihan | pelunasan
	CustomerID      int64      `db:"customer_id"`
	CustomerName    string     `db:"customer_name"`
	CustomerPhone   string     `db:"customer_phone"`
	CustomerAddress string     `db:"customer_address"`
	IssueDate       time.Time  `db:"issue_date"`
	DueDate         *time.Time `db:"due_date"`
	Notes           string     `db:"notes"`
	Total           int64      `db:"total"`
	CreatedByName   *string    `db:"created_by_name"`
	CreatedAt       time.Time  `db:"created_at"`
}

type InvoiceLine struct {
	OrderID    int64     `db:"order_id"`
	OrderCode  string    `db:"order_code"`
	OrderDate  time.Time `db:"order_date"`
	OrderTotal int64     `db:"order_total"`
	PaidBefore int64     `db:"paid_before"`
	AmountDue  int64     `db:"amount_due"`
	Items      string    `db:"items"`
}
