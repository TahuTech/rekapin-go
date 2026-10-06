package web

import (
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"rekapin/internal/store"
)

// dummyHash dipakai saat username tidak ada agar waktu respons tetap sama (cegah user enumeration).
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

func (a *App) loginForm(w http.ResponseWriter, r *http.Request) {
	if a.sessions.GetInt64(r.Context(), "userID") != 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.page(w, r, "auth/login", nil)
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.PostFormValue("username"))
	password := r.PostFormValue("password")

	u, err := a.st.UserByUsername(r.Context(), username)
	hash := dummyHash
	if err == nil {
		hash = []byte(u.PasswordHash)
	} else if !errors.Is(err, store.ErrNotFound) {
		a.serverError(w, r, err)
		return
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil || err != nil {
		a.page(w, r, "auth/login", D{"Error": "Username atau password salah", "Username": username})
		return
	}
	// Status akun & toko dicek setelah password valid agar tidak membocorkan username.
	if msg, err := a.loginBlocked(r, &u); err != nil {
		a.serverError(w, r, err)
		return
	} else if msg != "" {
		a.page(w, r, "auth/login", D{"Error": msg, "Username": username})
		return
	}
	// Ganti token session setelah login (cegah session fixation).
	if err := a.sessions.RenewToken(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.sessions.Put(r.Context(), "userID", u.ID)
	if u.IsMaster() {
		a.redirect(w, r, "/master")
		return
	}
	a.redirect(w, r, "/")
}

// loginBlocked mengembalikan alasan penolakan bila akun atau tokonya nonaktif.
func (a *App) loginBlocked(r *http.Request, u *store.User) (string, error) {
	if !u.Active {
		return "Akun Anda dinonaktifkan. Hubungi master.", nil
	}
	if u.IsMaster() || u.StoreID == nil {
		return "", nil
	}
	t, err := a.st.TenantByID(r.Context(), *u.StoreID)
	if errors.Is(err, store.ErrNotFound) || err == nil && !t.Active {
		return "Toko Anda sedang nonaktif. Hubungi master.", nil
	}
	return "", err
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	_ = a.sessions.Destroy(r.Context())
	a.redirect(w, r, "/login")
}

// HashPassword dipakai CLI & menu master saat membuat/mereset password.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}
