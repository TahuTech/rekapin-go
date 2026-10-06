package web

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"rekapin/internal/store"
)

func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	from, to := monthStart(today()), today()
	d, err := a.ts(r).Dashboard(ctx, from, to)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	debtors, err := a.ts(r).TopDebtors(ctx, 5)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	recent, _, _, err := a.ts(r).ListPayments(ctx, store.PaymentFilter{}, store.Page{Size: 8})
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.page(w, r, "dashboard/index", D{"D": d, "Debtors": debtors, "Recent": recent, "Month": from})
}

// csvWriter menyiapkan header download CSV. Separator ';' agar langsung rapi di Excel lokal Indonesia.
func csvWriter(w http.ResponseWriter, name string) *csv.Writer {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	_, _ = w.Write([]byte("\xEF\xBB\xBF")) // BOM UTF-8 untuk Excel
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	return cw
}

func i64(v int64) string { return strconv.FormatInt(v, 10) }

func rangeLabel(r store.DateRange) string {
	f, t := "awal", "sekarang"
	if r.From != nil {
		f = r.From.Format("20060102")
	}
	if r.To != nil {
		t = r.To.Format("20060102")
	}
	return f + "-" + t
}

func (a *App) reportCustomers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rng := dateRange(q, true)
	cid := formInt(q.Get("customer_id"))
	rows, err := a.ts(r).CustomerReport(r.Context(), rng, cid)
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	if q.Get("format") == "csv" {
		cw := csvWriter(w, "rekap-customer-"+rangeLabel(rng)+".csv")
		_ = cw.Write([]string{"Customer", "Jumlah Transaksi", "Total Beli", "Terbayar", "Sisa", "Penerimaan Periode"})
		for _, x := range rows {
			_ = cw.Write([]string{x.CustomerName, i64(x.OrderCount), i64(x.TotalBuy), i64(x.TotalPaid), i64(x.Remaining), i64(x.Receipts)})
		}
		cw.Flush()
		return
	}

	var tot store.CustomerReportRow
	for _, x := range rows {
		tot.OrderCount += x.OrderCount
		tot.TotalBuy += x.TotalBuy
		tot.TotalPaid += x.TotalPaid
		tot.Remaining += x.Remaining
		tot.Receipts += x.Receipts
	}
	customers, err := a.ts(r).CustomerOptions(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := D{"Rows": rows, "Tot": tot, "R": rng, "CustomerID": cid, "Customers": customers, "Query": q}
	if cid > 0 {
		products, err := a.ts(r).TopProducts(r.Context(), rng, cid, 20)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data["Products"] = products
	}
	a.page(w, r, "reports/customers", data)
}

func (a *App) reportPeriod(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rng := dateRange(q, true)
	// Periode report wajib berbatas agar generate_series tidak liar.
	from, to := monthStart(today()), today()
	if rng.From != nil {
		from = *rng.From
	}
	if rng.To != nil {
		to = *rng.To
	}
	if to.Before(from) {
		from, to = to, from
	}
	if to.Sub(from) > 5*366*24*time.Hour {
		from = to.AddDate(-5, 0, 0)
	}
	rng = store.DateRange{From: &from, To: &to}
	unit := q.Get("group")
	if unit != "month" {
		unit = "day"
	}

	rows, err := a.ts(r).PeriodReport(r.Context(), from, to, unit)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	layout := "2006-01-02"
	if unit == "month" {
		layout = "2006-01"
	}

	if q.Get("format") == "csv" {
		cw := csvWriter(w, "rekap-periode-"+rangeLabel(rng)+".csv")
		_ = cw.Write([]string{"Periode", "Jumlah Transaksi", "Penjualan", "Penerimaan"})
		for _, x := range rows {
			_ = cw.Write([]string{x.Bucket.Format(layout), i64(x.OrderCount), i64(x.Sales), i64(x.Receipts)})
		}
		cw.Flush()
		return
	}

	var tot store.PeriodRow
	var maxV int64 = 1
	for _, x := range rows {
		tot.OrderCount += x.OrderCount
		tot.Sales += x.Sales
		tot.Receipts += x.Receipts
		maxV = max(maxV, x.Sales, x.Receipts)
	}
	products, err := a.ts(r).TopProducts(r.Context(), rng, 0, 10)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.page(w, r, "reports/period", D{"Rows": rows, "Tot": tot, "Max": maxV, "R": rng, "Group": unit, "Products": products, "Query": q})
}
