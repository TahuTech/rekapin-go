package web

import (
	"net/http"
	"strconv"
	"strings"

	"rekapin/internal/money"
	"rekapin/internal/store"
)

func (a *App) paymentList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.PaymentFilter{
		CustomerID: formInt(q.Get("customer_id")),
		Method:     q.Get("method"),
		ShowVoided: q.Get("voided") == "1",
		DateRange:  dateRange(q, false),
	}
	list, total, sum, err := a.ts(r).ListPayments(r.Context(), f, store.Page{Num: pageNum(q), Size: pageSize})
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	customers, err := a.ts(r).CustomerOptions(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := D{"Payments": list, "Sum": sum, "F": f, "Customers": customers, "Pager": newPager(r, total, pageSize)}
	if r.Header.Get("HX-Target") == "payment-table" {
		a.render(w, r, http.StatusOK, "payments/index", "table", data)
		return
	}
	a.page(w, r, "payments/index", data)
}

func (a *App) paymentCreate(w http.ResponseWriter, r *http.Request) {
	orderID := pathID(r)
	amount, perr := money.Parse(r.PostFormValue("amount"))
	in := store.PaymentInput{
		OrderID: orderID,
		Amount:  amount,
		PaidAt:  parseDate(r.PostFormValue("paid_at")),
		Method:  r.PostFormValue("method"),
		Note:    strings.TrimSpace(r.PostFormValue("note")),
	}
	var msg string
	if perr != nil {
		msg = "Nominal tidak valid"
	} else {
		_, err := a.tsvc(r).AddPayment(r.Context(), in, userFrom(r.Context()).ID)
		var done bool
		if msg, done = a.handleErr(w, r, err); done {
			return
		}
		if err == nil {
			a.flash(r, "Pembayaran Rp "+money.Format(amount)+" tercatat")
			a.redirect(w, r, "/orders/"+strconv.FormatInt(orderID, 10))
			return
		}
	}
	a.renderOrderShow(w, r, orderID, D{"PayError": msg, "PayAmount": r.PostFormValue("amount"),
		"PayDate": in.PaidAt, "PayMethod": in.Method, "PayNote": in.Note})
}

func (a *App) paymentVoid(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	p, err := a.ts(r).PaymentByID(r.Context(), id)
	if _, done := a.handleErr(w, r, err); done {
		return
	}
	if err := a.tsvc(r).VoidPayment(r.Context(), id, userFrom(r.Context()).ID); err != nil {
		if _, done := a.handleErr(w, r, err); done {
			return
		}
	}
	a.flash(r, "Pembayaran dibatalkan (void)")
	back := r.PostFormValue("back")
	if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") {
		back = "/orders/" + strconv.FormatInt(p.OrderID, 10)
	}
	a.redirect(w, r, back)
}
