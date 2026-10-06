// Package web berisi HTTP handler, middleware, dan rendering template.
package web

import (
	"io/fs"
	"net/http"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"rekapin/internal/config"
	"rekapin/internal/service"
	"rekapin/internal/store"
)

type App struct {
	cfg      config.Config
	st       *store.Store
	svc      *service.Service
	sessions *scs.SessionManager
	tpl      templates
	static   fs.FS
}

func New(cfg config.Config, pool *pgxpool.Pool, assets fs.FS) (*App, error) {
	tpl, err := parseTemplates(assets)
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}

	sm := scs.New()
	// Cleanup session kedaluwarsa tiap 30 menit (cukup untuk trafik admin).
	sm.Store = pgxstore.NewWithCleanupInterval(pool, 30*time.Minute)
	sm.Lifetime = 7 * 24 * time.Hour
	sm.IdleTimeout = 12 * time.Hour
	sm.Cookie.Name = "rekapin_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = cfg.SecureCookie

	st := store.New(pool)
	return &App{cfg: cfg, st: st, svc: service.New(st), sessions: sm, tpl: tpl, static: static}, nil
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()

	// Static: cache panjang; nama file di-versi lewat query ?v= pada layout.
	staticHandler := http.StripPrefix("/static/", http.FileServerFS(a.static))
	mux.Handle("GET /static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		staticHandler.ServeHTTP(w, r)
	}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })

	mux.HandleFunc("GET /login", a.loginForm)
	mux.HandleFunc("POST /login", a.login)

	// Semua route di bawah ini wajib login.
	p := http.NewServeMux()
	p.HandleFunc("POST /logout", a.logout)
	p.HandleFunc("GET /{$}", a.dashboard)

	p.HandleFunc("GET /customers", a.customerList)
	p.HandleFunc("GET /customers/new", a.customerNew)
	p.HandleFunc("POST /customers", a.customerCreate)
	p.HandleFunc("GET /customers/{id}", a.customerShow)
	p.HandleFunc("GET /customers/{id}/edit", a.customerEdit)
	p.HandleFunc("POST /customers/{id}", a.customerUpdate)
	p.HandleFunc("POST /customers/{id}/archive", a.customerArchive)

	p.HandleFunc("GET /products", a.productList)
	p.HandleFunc("GET /products/new", a.productNew)
	p.HandleFunc("POST /products", a.productCreate)
	p.HandleFunc("GET /products/{id}/edit", a.productEdit)
	p.HandleFunc("POST /products/{id}", a.productUpdate)
	p.HandleFunc("POST /products/{id}/toggle", a.productToggle)

	p.HandleFunc("GET /orders", a.orderList)
	p.HandleFunc("GET /orders/new", a.orderNew)
	p.HandleFunc("GET /orders/item-row", a.orderItemRow)
	p.HandleFunc("POST /orders", a.orderCreate)
	p.HandleFunc("GET /orders/{id}", a.orderShow)
	p.HandleFunc("GET /orders/{id}/edit", a.orderEdit)
	p.HandleFunc("POST /orders/{id}", a.orderUpdate)
	p.HandleFunc("POST /orders/{id}/delete", a.orderDelete)
	p.HandleFunc("POST /orders/{id}/payments", a.paymentCreate)

	p.HandleFunc("GET /payments", a.paymentList)
	p.HandleFunc("POST /payments/{id}/void", a.paymentVoid)

	p.HandleFunc("GET /invoices", a.invoiceList)
	p.HandleFunc("GET /invoices/new", a.invoiceNew)
	p.HandleFunc("GET /invoices/outstanding", a.invoiceOutstanding)
	p.HandleFunc("POST /invoices", a.invoiceCreate)
	p.HandleFunc("GET /invoices/{id}", a.invoiceShow)
	p.HandleFunc("GET /invoices/{id}/print", a.invoicePrint)

	p.HandleFunc("GET /reports/customers", a.reportCustomers)
	p.HandleFunc("GET /reports/period", a.reportPeriod)

	mux.Handle("/", a.requireAuth(p))

	// CrossOriginProtection (Go 1.25) menolak POST lintas origin -> proteksi CSRF tanpa token.
	csrf := http.NewCrossOriginProtection()
	return a.recoverPanic(a.logRequest(csrf.Handler(a.sessions.LoadAndSave(securityHeaders(mux)))))
}
