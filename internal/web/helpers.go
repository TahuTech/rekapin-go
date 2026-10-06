package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"rekapin/internal/service"
	"rekapin/internal/store"
)

func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func formInt(v string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	return n
}

// parseDate menerima format input HTML "2006-01-02"; kosong/invalid -> zero time.
func parseDate(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseDatePtr(s string) *time.Time {
	t := parseDate(s)
	if t.IsZero() {
		return nil
	}
	return &t
}

func today() time.Time {
	y, m, d := time.Now().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local)
}

// dateRange membaca ?from=&to= ; bila withDefault, default = awal bulan s/d hari ini.
func dateRange(q url.Values, withDefault bool) store.DateRange {
	r := store.DateRange{From: parseDatePtr(q.Get("from")), To: parseDatePtr(q.Get("to"))}
	if withDefault && r.From == nil && r.To == nil {
		f, t := monthStart(today()), today()
		r.From, r.To = &f, &t
	}
	return r
}

func pageNum(q url.Values) int {
	n, _ := strconv.Atoi(q.Get("page"))
	if n < 1 {
		return 1
	}
	return n
}

// Pager data untuk partial pagination.
type Pager struct {
	Page, Pages, Total int
	Base               string
	Query              url.Values
}

func newPager(r *http.Request, total, size int) Pager {
	pages := (total + size - 1) / size
	if pages < 1 {
		pages = 1
	}
	return Pager{Page: pageNum(r.URL.Query()), Pages: pages, Total: total, Base: r.URL.Path, Query: r.URL.Query()}
}

func (p Pager) HasPrev() bool { return p.Page > 1 }
func (p Pager) HasNext() bool { return p.Page < p.Pages }
func (p Pager) Prev() int     { return p.Page - 1 }
func (p Pager) Next() int     { return p.Page + 1 }

// handleErr: validasi -> pesan ke user, not found -> 404, lainnya -> 500.
// Mengembalikan pesan validasi (bila ada) agar handler bisa re-render form.
func (a *App) handleErr(w http.ResponseWriter, r *http.Request, err error) (msg string, handled bool) {
	switch {
	case err == nil:
		return "", false
	case service.IsValidation(err):
		return err.Error(), false
	case errors.Is(err, store.ErrNotFound):
		http.NotFound(w, r)
		return "", true
	default:
		a.serverError(w, r, err)
		return "", true
	}
}

const pageSize = 25
