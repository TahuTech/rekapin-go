package web

import (
	"net/http"
	"strings"

	"rekapin/internal/money"
	"rekapin/internal/store"
)

func (a *App) productList(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	list, err := a.st.ListProducts(r.Context(), q, false)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := D{"Products": list, "Q": q}
	if r.Header.Get("HX-Target") == "product-table" {
		a.render(w, r, http.StatusOK, "products/index", "table", data)
		return
	}
	a.page(w, r, "products/index", data)
}

func (a *App) productNew(w http.ResponseWriter, r *http.Request) {
	a.page(w, r, "products/form", D{"Product": store.Product{Unit: "pcs", Active: true}})
}

func productInput(r *http.Request) (store.ProductInput, string) {
	in := store.ProductInput{
		Name:   strings.TrimSpace(r.PostFormValue("name")),
		Unit:   strings.TrimSpace(r.PostFormValue("unit")),
		Active: r.PostFormValue("active") == "on",
	}
	price, err := money.Parse(r.PostFormValue("price"))
	switch {
	case in.Name == "":
		return in, "Nama barang wajib diisi"
	case err != nil || price < 0:
		return in, "Harga tidak valid"
	}
	in.Price = price
	return in, ""
}

func (a *App) productCreate(w http.ResponseWriter, r *http.Request) {
	in, msg := productInput(r)
	if msg != "" {
		a.page(w, r, "products/form", D{"Error": msg, "Product": store.Product{Name: in.Name, Unit: in.Unit, Price: in.Price, Active: in.Active}})
		return
	}
	if _, err := a.st.CreateProduct(r.Context(), in); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.flash(r, "Barang berhasil ditambahkan")
	a.redirect(w, r, "/products")
}

func (a *App) productEdit(w http.ResponseWriter, r *http.Request) {
	p, err := a.st.ProductByID(r.Context(), pathID(r))
	if _, done := a.handleErr(w, r, err); done {
		return
	}
	a.page(w, r, "products/form", D{"Product": p})
}

func (a *App) productUpdate(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	in, msg := productInput(r)
	if msg != "" {
		a.page(w, r, "products/form", D{"Error": msg, "Product": store.Product{ID: id, Name: in.Name, Unit: in.Unit, Price: in.Price, Active: in.Active}})
		return
	}
	if _, done := a.handleErr(w, r, a.st.UpdateProduct(r.Context(), id, in)); done {
		return
	}
	a.flash(r, "Barang diperbarui")
	a.redirect(w, r, "/products")
}

func (a *App) productToggle(w http.ResponseWriter, r *http.Request) {
	if err := a.st.ToggleProduct(r.Context(), pathID(r)); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirect(w, r, "/products")
}
