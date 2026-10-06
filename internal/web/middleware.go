package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"rekapin/internal/service"
	"rekapin/internal/store"
)

type ctxKey int

const (
	userKey ctxKey = iota + 1
	tenantKey
)

// actingStoreKey = key session berisi toko yang sedang dibuka oleh master.
const actingStoreKey = "actingStoreID"

func userFrom(ctx context.Context) *store.User {
	u, _ := ctx.Value(userKey).(*store.User)
	return u
}

// tenantFrom = toko aktif request ini; nil di halaman master / login.
func tenantFrom(ctx context.Context) *store.Tenant {
	t, _ := ctx.Value(tenantKey).(*store.Tenant)
	return t
}

// ts = Store yang dibatasi ke toko aktif. Hanya valid di route yang dibungkus withTenant.
func (a *App) ts(r *http.Request) *store.Store {
	return a.st.ForStore(tenantFrom(r.Context()).ID)
}

// tsvc = Service yang dibatasi ke toko aktif.
func (a *App) tsvc(r *http.Request) *service.Service {
	return a.svc.ForStore(tenantFrom(r.Context()).ID)
}

func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := a.sessions.GetInt64(r.Context(), "userID")
		if id == 0 {
			a.redirect(w, r, "/login")
			return
		}
		u, err := a.st.UserByID(r.Context(), id)
		if err != nil || !u.Active {
			// User dihapus / dinonaktifkan / session basi -> paksa login ulang.
			_ = a.sessions.Destroy(r.Context())
			a.redirect(w, r, "/login")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, &u)))
	})
}

// withTenant menentukan toko aktif: admin selalu tokonya sendiri, master memakai toko
// yang dipilih lewat menu master. Dipasang setelah requireAuth.
func (a *App) withTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		var id int64
		if u.IsMaster() {
			id = a.sessions.GetInt64(r.Context(), actingStoreKey)
			if id == 0 {
				a.redirect(w, r, "/master")
				return
			}
		} else if u.StoreID != nil {
			id = *u.StoreID
		}
		t, err := a.st.TenantByID(r.Context(), id)
		switch {
		case err == nil && (t.Active || u.IsMaster()):
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tenantKey, &t)))
		case u.IsMaster():
			// Toko yang dipilih sudah dihapus -> kembali ke daftar toko.
			a.sessions.Remove(r.Context(), actingStoreKey)
			a.redirect(w, r, "/master")
		case err == nil || errors.Is(err, store.ErrNotFound):
			// Toko nonaktif / tidak ada -> admin tidak boleh masuk.
			_ = a.sessions.Destroy(r.Context())
			a.redirect(w, r, "/login")
		default:
			a.serverError(w, r, err)
		}
	})
}

// requireMaster membatasi route hanya untuk role master. Dipasang setelah requireAuth.
func (a *App) requireMaster(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !userFrom(r.Context()).IsMaster() {
			http.Error(w, "Akses khusus master", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (a *App) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.URL.Path == "/healthz" || len(r.URL.Path) > 7 && r.URL.Path[:8] == "/static/" {
			return
		}
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "dur", time.Since(start).Round(time.Microsecond))
	})
}

func (a *App) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				w.Header().Set("Connection", "close")
				a.serverError(w, r, fmt.Errorf("panic: %v", v))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
