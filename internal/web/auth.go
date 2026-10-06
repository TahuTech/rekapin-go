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
	// Ganti token session setelah login (cegah session fixation).
	if err := a.sessions.RenewToken(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.sessions.Put(r.Context(), "userID", u.ID)
	a.redirect(w, r, "/")
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	_ = a.sessions.Destroy(r.Context())
	a.redirect(w, r, "/login")
}

// HashPassword dipakai CLI pembuatan admin.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}
