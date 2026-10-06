package web

import (
	"net/http"
	"strconv"
	"strings"

	"rekapin/internal/money"
	"rekapin/internal/store"
)

func (a *App) orderList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.OrderFilter{
		CustomerID: formInt(q.Get("customer_id")),
		Status:     q.Get("status"),
		Q:          strings.TrimSpace(q.Get("q")),
		DateRange:  dateRange(q, false),
	}
	list, total, err := a.ts(r).ListOrders(r.Context(), f, store.Page{Num: pageNum(q), Size: pageSize})
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	customers, err := a.ts(r).CustomerOptions(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := D{"Orders": list, "F": f, "Customers": customers, "Pager": newPager(r, total, pageSize)}
	if r.Header.Get("HX-Target") == "order-table" {
		a.render(w, r, http.StatusOK, "orders/index", "table", data)
		return
	}
	a.page(w, r, "orders/index", data)
}

// orderFormData memuat data dropdown untuk form transaksi.
func (a *App) orderFormData(r *http.Request, data D) (D, error) {
	customers, err := a.ts(r).CustomerOptions(r.Context())
	if err != nil {
		return nil, err
	}
	products, err := a.ts(r).ListProducts(r.Context(), "", true)
	if err != nil {
		return nil, err
	}
	data["Customers"], data["Products"] = customers, products
	return data, nil
}

func (a *App) orderNew(w http.ResponseWriter, r *http.Request) {
	in := store.OrderInput{
		CustomerID: formInt(r.URL.Query().Get("customer_id")),
		OrderDate:  today(),
		Items:      []store.OrderItemInput{{Qty: 1}},
	}
	data, err := a.orderFormData(r, D{"In": in, "DPMethod": "cash"})
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.page(w, r, "orders/form", data)
}

// orderItemRow mengembalikan satu baris item kosong (ditambahkan via hx-swap="beforeend").
func (a *App) orderItemRow(w http.ResponseWriter, r *http.Request) {
	products, err := a.ts(r).ListProducts(r.Context(), "", true)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "orders/form", "item-row", D{"Item": store.OrderItemInput{Qty: 1}, "Products": products})
}

// parseOrderForm membaca header + array item dari form. Error parsing angka
// dikembalikan sebagai pesan agar form bisa ditampilkan ulang.
func parseOrderForm(r *http.Request) (store.OrderInput, string) {
	_ = r.ParseForm()
	f := r.PostForm
	in := store.OrderInput{
		CustomerID: formInt(f.Get("customer_id")),
		OrderDate:  parseDate(f.Get("order_date")),
		Notes:      strings.TrimSpace(f.Get("notes")),
	}
	names, units, qtys, prices, pids := f["product_name"], f["unit"], f["qty"], f["price"], f["product_id"]
	var msg string
	for i := range names {
		it := store.OrderItemInput{ProductName: strings.TrimSpace(names[i])}
		if i < len(units) {
			it.Unit = strings.TrimSpace(units[i])
		}
		if i < len(pids) {
			if id := formInt(pids[i]); id > 0 {
				it.ProductID = &id
			}
		}
		if i < len(qtys) {
			q, err := money.ParseQty(qtys[i])
			if err != nil && it.ProductName != "" && msg == "" {
				msg = "Qty untuk " + it.ProductName + " tidak valid"
			}
			it.Qty = q
		}
		if i < len(prices) {
			p, err := money.Parse(prices[i])
			if err != nil && it.ProductName != "" && msg == "" {
				msg = "Harga untuk " + it.ProductName + " tidak valid"
			}
			it.UnitPrice = p
		}
		in.Items = append(in.Items, it)
	}
	return in, msg
}

func (a *App) orderCreate(w http.ResponseWriter, r *http.Request) {
	in, msg := parseOrderForm(r)
	dp, err := money.Parse(r.PostFormValue("dp_amount"))
	if err != nil && msg == "" {
		msg = "Nominal uang muka tidak valid"
	}
	dpMethod := r.PostFormValue("dp_method")
	if msg == "" {
		id, err := a.tsvc(r).CreateOrder(r.Context(), in, dp, dpMethod, userFrom(r.Context()).ID)
		if msg, _ = a.handleErr(w, r, err); err == nil {
			a.flash(r, "Transaksi berhasil disimpan")
			a.redirect(w, r, "/orders/"+strconv.FormatInt(id, 10))
			return
		} else if msg == "" {
			return // sudah ditangani (500)
		}
	}
	a.rerenderOrderForm(w, r, D{"In": in, "Error": msg, "DP": dp, "DPMethod": dpMethod})
}

func (a *App) rerenderOrderForm(w http.ResponseWriter, r *http.Request, data D) {
	if in := data["In"].(store.OrderInput); len(in.Items) == 0 {
		in.Items = []store.OrderItemInput{{Qty: 1}}
		data["In"] = in
	}
	data, err := a.orderFormData(r, data)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.page(w, r, "orders/form", data)
}

func (a *App) orderShow(w http.ResponseWriter, r *http.Request) {
	a.renderOrderShow(w, r, pathID(r), D{})
}

func (a *App) renderOrderShow(w http.ResponseWriter, r *http.Request, id int64, data D) {
	ctx := r.Context()
	o, err := a.ts(r).OrderByID(ctx, id)
	if _, done := a.handleErr(w, r, err); done {
		return
	}
	items, err := a.ts(r).OrderItems(ctx, id)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	payments, _, _, err := a.ts(r).ListPayments(ctx, store.PaymentFilter{OrderID: id, ShowVoided: true}, store.Page{Size: 500})
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	locked, err := a.ts(r).OrderHasHistory(ctx, id)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data["O"], data["Items"], data["Payments"], data["Locked"] = o, items, payments, locked
	if _, ok := data["PayDate"]; !ok {
		data["PayDate"] = today()
	}
	a.page(w, r, "orders/show", data)
}

func (a *App) orderEdit(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	o, err := a.ts(r).OrderByID(r.Context(), id)
	if _, done := a.handleErr(w, r, err); done {
		return
	}
	items, err := a.ts(r).OrderItems(r.Context(), id)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	in := store.OrderInput{CustomerID: o.CustomerID, OrderDate: o.OrderDate, Notes: o.Notes}
	for _, it := range items {
		in.Items = append(in.Items, store.OrderItemInput{
			ProductID: it.ProductID, ProductName: it.ProductName, Unit: it.Unit, Qty: it.Qty, UnitPrice: it.UnitPrice,
		})
	}
	a.rerenderOrderForm(w, r, D{"In": in, "OrderID": id, "Code": o.Code})
}

func (a *App) orderUpdate(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	in, msg := parseOrderForm(r)
	if msg == "" {
		err := a.tsvc(r).UpdateOrder(r.Context(), id, in)
		if msg, _ = a.handleErr(w, r, err); err == nil {
			a.flash(r, "Transaksi diperbarui")
			a.redirect(w, r, "/orders/"+strconv.FormatInt(id, 10))
			return
		} else if msg == "" {
			return
		}
	}
	a.rerenderOrderForm(w, r, D{"In": in, "OrderID": id, "Error": msg, "Code": r.PostFormValue("code")})
}

func (a *App) orderDelete(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	err := a.tsvc(r).DeleteOrder(r.Context(), id)
	if msg, done := a.handleErr(w, r, err); done {
		return
	} else if msg != "" {
		a.flash(r, msg)
		a.redirect(w, r, "/orders/"+strconv.FormatInt(id, 10))
		return
	}
	a.flash(r, "Transaksi dihapus")
	a.redirect(w, r, "/orders")
}
