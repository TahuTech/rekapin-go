package web

import (
	"net/http"
	"strconv"
	"strings"

	"rekapin/internal/store"
)

// ---------- Toko ----------

func (a *App) masterStores(w http.ResponseWriter, r *http.Request) {
	list, err := a.st.ListTenants(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.page(w, r, "master/stores", D{"Stores": list, "ActingID": a.sessions.GetInt64(r.Context(), actingStoreKey)})
}

func (a *App) masterStoreNew(w http.ResponseWriter, r *http.Request) {
	a.page(w, r, "master/store_form", D{"Store": store.Tenant{Active: true}})
}

func tenantInput(r *http.Request) (store.TenantInput, string) {
	in := store.TenantInput{
		Name:   strings.TrimSpace(r.PostFormValue("name")),
		Info:   strings.TrimSpace(r.PostFormValue("info")),
		Active: r.PostFormValue("active") == "on",
	}
	if in.Name == "" {
		return in, "Nama toko wajib diisi"
	}
	return in, ""
}

func (a *App) masterStoreCreate(w http.ResponseWriter, r *http.Request) {
	in, msg := tenantInput(r)
	if msg != "" {
		a.page(w, r, "master/store_form", D{"Error": msg, "Store": store.Tenant{Name: in.Name, Info: in.Info, Active: in.Active}})
		return
	}
	id, err := a.st.CreateTenant(r.Context(), in)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.flash(r, "Toko berhasil dibuat. Tambahkan admin untuk toko ini.")
	a.redirect(w, r, "/master/stores/"+strconv.FormatInt(id, 10))
}

func (a *App) masterStoreShow(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	t, err := a.st.TenantByID(r.Context(), id)
	if _, done := a.handleErr(w, r, err); done {
		return
	}
	users, err := a.st.ListUsers(r.Context(), id)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.page(w, r, "master/store_show", D{"Store": t, "Users": users})
}

func (a *App) masterStoreEdit(w http.ResponseWriter, r *http.Request) {
	t, err := a.st.TenantByID(r.Context(), pathID(r))
	if _, done := a.handleErr(w, r, err); done {
		return
	}
	a.page(w, r, "master/store_form", D{"Store": t})
}

func (a *App) masterStoreUpdate(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	in, msg := tenantInput(r)
	if msg != "" {
		a.page(w, r, "master/store_form", D{"Error": msg, "Store": store.Tenant{ID: id, Name: in.Name, Info: in.Info, Active: in.Active}})
		return
	}
	if _, done := a.handleErr(w, r, a.st.UpdateTenant(r.Context(), id, in)); done {
		return
	}
	a.flash(r, "Toko diperbarui")
	a.redirect(w, r, "/master/stores/"+strconv.FormatInt(id, 10))
}

func (a *App) masterStoreToggle(w http.ResponseWriter, r *http.Request) {
	if _, done := a.handleErr(w, r, a.st.ToggleTenant(r.Context(), pathID(r))); done {
		return
	}
	a.redirect(w, r, "/master")
}

// masterStoreEnter membuka data operasional toko sebagai master (untuk support/pengecekan).
func (a *App) masterStoreEnter(w http.ResponseWriter, r *http.Request) {
	t, err := a.st.TenantByID(r.Context(), pathID(r))
	if _, done := a.handleErr(w, r, err); done {
		return
	}
	a.sessions.Put(r.Context(), actingStoreKey, t.ID)
	a.flash(r, "Anda sedang membuka toko "+t.Name)
	a.redirect(w, r, "/")
}

func (a *App) masterStoreExit(w http.ResponseWriter, r *http.Request) {
	a.sessions.Remove(r.Context(), actingStoreKey)
	a.redirect(w, r, "/master")
}

// ---------- Pengguna ----------

func (a *App) masterUsers(w http.ResponseWriter, r *http.Request) {
	list, err := a.st.ListUsers(r.Context(), 0)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.page(w, r, "master/users", D{"Users": list})
}

// userForm merender form user beserta daftar toko untuk dropdown.
func (a *App) userForm(w http.ResponseWriter, r *http.Request, u store.User, msg string) {
	stores, err := a.st.ListTenants(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.page(w, r, "master/user_form", D{"U": u, "Stores": stores, "Error": msg, "Self": u.ID == userFrom(r.Context()).ID})
}

func (a *App) masterUserNew(w http.ResponseWriter, r *http.Request) {
	u := store.User{Role: store.RoleAdmin, Active: true}
	if sid := formInt(r.URL.Query().Get("store")); sid > 0 {
		u.StoreID = &sid
	}
	a.userForm(w, r, u, "")
}

// userInput membaca & memvalidasi form user. isNew mewajibkan password.
func userInput(r *http.Request, isNew bool) (store.UserInput, string, string) {
	in := store.UserInput{
		Username: strings.ToLower(strings.TrimSpace(r.PostFormValue("username"))),
		Name:     strings.TrimSpace(r.PostFormValue("name")),
		Role:     r.PostFormValue("role"),
		Active:   r.PostFormValue("active") == "on",
	}
	if sid := formInt(r.PostFormValue("store_id")); sid > 0 && in.Role == store.RoleAdmin {
		in.StoreID = &sid
	}
	pw := r.PostFormValue("password")
	switch {
	case in.Username == "" || strings.ContainsAny(in.Username, " \t"):
		return in, pw, "Username wajib diisi dan tanpa spasi"
	case in.Name == "":
		return in, pw, "Nama wajib diisi"
	case in.Role != store.RoleAdmin && in.Role != store.RoleMaster:
		return in, pw, "Role tidak valid"
	case in.Role == store.RoleAdmin && in.StoreID == nil:
		return in, pw, "Admin wajib dipilihkan toko"
	case (isNew || pw != "") && len(pw) < 8:
		return in, pw, "Password minimal 8 karakter"
	}
	return in, pw, ""
}

func formUser(id int64, in store.UserInput) store.User {
	return store.User{ID: id, Username: in.Username, Name: in.Name, Role: in.Role, StoreID: in.StoreID, Active: in.Active}
}

func (a *App) masterUserCreate(w http.ResponseWriter, r *http.Request) {
	in, pw, msg := userInput(r, true)
	if msg != "" {
		a.userForm(w, r, formUser(0, in), msg)
		return
	}
	hash, err := HashPassword(pw)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if _, err := a.st.InsertUser(r.Context(), in, hash); store.IsUniqueViolation(err) {
		a.userForm(w, r, formUser(0, in), "Username sudah dipakai")
		return
	} else if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.flash(r, "Pengguna "+in.Username+" berhasil dibuat")
	a.redirectAfterUser(w, r, in)
}

func (a *App) masterUserEdit(w http.ResponseWriter, r *http.Request) {
	u, err := a.st.UserByID(r.Context(), pathID(r))
	if _, done := a.handleErr(w, r, err); done {
		return
	}
	a.userForm(w, r, u, "")
}

func (a *App) masterUserUpdate(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	in, pw, msg := userInput(r, false)
	// Master tidak boleh mengunci dirinya sendiri (menonaktifkan / menurunkan role).
	if id == userFrom(r.Context()).ID && msg == "" && (in.Role != store.RoleMaster || !in.Active) {
		msg = "Anda tidak dapat menonaktifkan atau mengubah role akun sendiri"
	}
	if msg != "" {
		a.userForm(w, r, formUser(id, in), msg)
		return
	}
	err := a.st.WithTx(r.Context(), func(tx *store.Store) error {
		if err := tx.UpdateUser(r.Context(), id, in); err != nil {
			return err
		}
		if pw == "" {
			return nil
		}
		hash, err := HashPassword(pw)
		if err != nil {
			return err
		}
		return tx.SetUserPassword(r.Context(), id, hash)
	})
	if store.IsUniqueViolation(err) {
		a.userForm(w, r, formUser(id, in), "Username sudah dipakai")
		return
	}
	if _, done := a.handleErr(w, r, err); done {
		return
	}
	a.flash(r, "Pengguna "+in.Username+" diperbarui")
	a.redirectAfterUser(w, r, in)
}

// redirectAfterUser kembali ke halaman toko si admin, atau ke daftar pengguna untuk master.
func (a *App) redirectAfterUser(w http.ResponseWriter, r *http.Request, in store.UserInput) {
	if in.StoreID != nil {
		a.redirect(w, r, "/master/stores/"+strconv.FormatInt(*in.StoreID, 10))
		return
	}
	a.redirect(w, r, "/master/users")
}
