package web

import (
	"net/http"
	"strconv"
	"strings"

	"rekapin/internal/store"
)

func (a *App) customerList(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	list, total, err := a.ts(r).ListCustomers(r.Context(), q, store.Page{Num: pageNum(r.URL.Query()), Size: pageSize})
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := D{"Customers": list, "Q": q, "Pager": newPager(r, total, pageSize)}
	// Pencarian live hanya mengganti tabel.
	if r.Header.Get("HX-Target") == "customer-table" {
		a.render(w, r, http.StatusOK, "customers/index", "table", data)
		return
	}
	a.page(w, r, "customers/index", data)
}

func (a *App) customerNew(w http.ResponseWriter, r *http.Request) {
	a.page(w, r, "customers/form", D{"Customer": store.Customer{}})
}

func customerInput(r *http.Request) store.CustomerInput {
	return store.CustomerInput{
		Name:    strings.TrimSpace(r.PostFormValue("name")),
		Phone:   strings.TrimSpace(r.PostFormValue("phone")),
		Address: strings.TrimSpace(r.PostFormValue("address")),
		Notes:   strings.TrimSpace(r.PostFormValue("notes")),
	}
}

func (a *App) customerCreate(w http.ResponseWriter, r *http.Request) {
	in := customerInput(r)
	if in.Name == "" {
		a.page(w, r, "customers/form", D{"Error": "Nama wajib diisi", "Customer": store.Customer{Name: in.Name, Phone: in.Phone, Address: in.Address, Notes: in.Notes}})
		return
	}
	id, err := a.ts(r).CreateCustomer(r.Context(), in)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.flash(r, "Customer berhasil ditambahkan")
	a.redirect(w, r, "/customers/"+strconv.FormatInt(id, 10))
}

func (a *App) customerShow(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	ctx := r.Context()
	c, err := a.ts(r).CustomerSummaryByID(ctx, id)
	if _, done := a.handleErr(w, r, err); done {
		return
	}
	q := r.URL.Query()
	tab := q.Get("tab")
	data := D{"C": c, "Tab": tab}
	pg := store.Page{Num: pageNum(q), Size: pageSize}
	switch tab {
	case "payments":
		list, total, sum, err := a.ts(r).ListPayments(ctx, store.PaymentFilter{CustomerID: id}, pg)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data["Payments"], data["PaymentSum"], data["Pager"] = list, sum, newPager(r, total, pageSize)
	case "invoices":
		list, total, err := a.ts(r).ListInvoices(ctx, store.InvoiceFilter{CustomerID: id}, pg)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data["Invoices"], data["Pager"] = list, newPager(r, total, pageSize)
	default:
		data["Tab"] = "orders"
		list, total, err := a.ts(r).ListOrders(ctx, store.OrderFilter{CustomerID: id, Status: q.Get("status")}, pg)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data["Orders"], data["Pager"], data["Status"] = list, newPager(r, total, pageSize), q.Get("status")
	}
	a.page(w, r, "customers/show", data)
}

func (a *App) customerEdit(w http.ResponseWriter, r *http.Request) {
	c, err := a.ts(r).CustomerByID(r.Context(), pathID(r))
	if _, done := a.handleErr(w, r, err); done {
		return
	}
	a.page(w, r, "customers/form", D{"Customer": c})
}

func (a *App) customerUpdate(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	in := customerInput(r)
	if in.Name == "" {
		a.page(w, r, "customers/form", D{"Error": "Nama wajib diisi", "Customer": store.Customer{ID: id, Name: in.Name, Phone: in.Phone, Address: in.Address, Notes: in.Notes}})
		return
	}
	if _, done := a.handleErr(w, r, a.ts(r).UpdateCustomer(r.Context(), id, in)); done {
		return
	}
	a.flash(r, "Data customer diperbarui")
	a.redirect(w, r, "/customers/"+strconv.FormatInt(id, 10))
}

func (a *App) customerArchive(w http.ResponseWriter, r *http.Request) {
	if err := a.ts(r).ArchiveCustomer(r.Context(), pathID(r)); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.flash(r, "Customer diarsipkan")
	a.redirect(w, r, "/customers")
}
