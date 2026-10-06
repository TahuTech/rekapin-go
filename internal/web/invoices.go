package web

import (
	"net/http"
	"strconv"
	"strings"

	"rekapin/internal/service"
	"rekapin/internal/store"
)

func (a *App) invoiceList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.InvoiceFilter{CustomerID: formInt(q.Get("customer_id")), Type: q.Get("type"), DateRange: dateRange(q, false)}
	list, total, err := a.ts(r).ListInvoices(r.Context(), f, store.Page{Num: pageNum(q), Size: pageSize})
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	customers, err := a.ts(r).CustomerOptions(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := D{"Invoices": list, "F": f, "Customers": customers, "Pager": newPager(r, total, pageSize)}
	if r.Header.Get("HX-Target") == "invoice-table" {
		a.render(w, r, http.StatusOK, "invoices/index", "table", data)
		return
	}
	a.page(w, r, "invoices/index", data)
}

func invoiceType(s string) string {
	if s == "pelunasan" {
		return s
	}
	return "tagihan"
}

func (a *App) invoiceNew(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req := service.InvoiceRequest{
		InvoiceInput: store.InvoiceInput{Type: invoiceType(q.Get("type")), CustomerID: formInt(q.Get("customer_id")), IssueDate: today()},
		Method:       "cash",
	}
	if oid := formInt(q.Get("order_id")); oid > 0 {
		req.OrderIDs = []int64{oid}
	}
	if req.Type == "tagihan" {
		due := today().AddDate(0, 0, 14)
		req.DueDate = &due
	}
	a.renderInvoiceForm(w, r, req, "")
}

func (a *App) renderInvoiceForm(w http.ResponseWriter, r *http.Request, req service.InvoiceRequest, msg string) {
	customers, err := a.ts(r).CustomerOptions(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := D{"Req": req, "Customers": customers, "Error": msg, "Selected": selectedSet(req.OrderIDs), "SelectAll": len(req.OrderIDs) == 0}
	if req.CustomerID > 0 {
		orders, err := a.ts(r).OutstandingOrders(r.Context(), req.CustomerID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data["Orders"] = orders
	}
	a.page(w, r, "invoices/form", data)
}

func selectedSet(ids []int64) map[int64]bool {
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// invoiceOutstanding = partial daftar transaksi belum lunas saat customer dipilih.
func (a *App) invoiceOutstanding(w http.ResponseWriter, r *http.Request) {
	cid := formInt(r.URL.Query().Get("customer_id"))
	data := D{"Selected": map[int64]bool{}, "SelectAll": true}
	if cid > 0 {
		orders, err := a.ts(r).OutstandingOrders(r.Context(), cid)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data["Orders"] = orders
		data["Req"] = service.InvoiceRequest{InvoiceInput: store.InvoiceInput{CustomerID: cid}}
	}
	a.render(w, r, http.StatusOK, "invoices/form", "outstanding", data)
}

func (a *App) invoiceCreate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f := r.PostForm
	req := service.InvoiceRequest{
		InvoiceInput: store.InvoiceInput{
			Type:       invoiceType(f.Get("type")),
			CustomerID: formInt(f.Get("customer_id")),
			IssueDate:  parseDate(f.Get("issue_date")),
			DueDate:    parseDatePtr(f.Get("due_date")),
			Notes:      strings.TrimSpace(f.Get("notes")),
		},
		Method: f.Get("method"),
	}
	for _, v := range f["order_id"] {
		if id := formInt(v); id > 0 {
			req.OrderIDs = append(req.OrderIDs, id)
		}
	}
	id, err := a.tsvc(r).CreateInvoice(r.Context(), req, userFrom(r.Context()).ID)
	msg, done := a.handleErr(w, r, err)
	if done {
		return
	}
	if msg != "" {
		a.renderInvoiceForm(w, r, req, msg)
		return
	}
	if req.Type == "pelunasan" {
		a.flash(r, "Pelunasan tercatat dan kwitansi dibuat")
	} else {
		a.flash(r, "Invoice tagihan dibuat")
	}
	a.redirect(w, r, "/invoices/"+strconv.FormatInt(id, 10))
}

func (a *App) invoiceData(w http.ResponseWriter, r *http.Request) (D, bool) {
	id := pathID(r)
	inv, err := a.ts(r).InvoiceByID(r.Context(), id)
	if _, done := a.handleErr(w, r, err); done {
		return nil, false
	}
	lines, err := a.ts(r).InvoiceLines(r.Context(), id)
	if err != nil {
		a.serverError(w, r, err)
		return nil, false
	}
	payments, err := a.ts(r).InvoicePayments(r.Context(), id)
	if err != nil {
		a.serverError(w, r, err)
		return nil, false
	}
	return D{"Inv": inv, "Lines": lines, "Payments": payments}, true
}

func (a *App) invoiceShow(w http.ResponseWriter, r *http.Request) {
	if data, ok := a.invoiceData(w, r); ok {
		a.page(w, r, "invoices/show", data)
	}
}

func (a *App) invoicePrint(w http.ResponseWriter, r *http.Request) {
	if data, ok := a.invoiceData(w, r); ok {
		a.render(w, r, http.StatusOK, "invoices/print", "print", data)
	}
}
