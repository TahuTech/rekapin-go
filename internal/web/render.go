package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"rekapin/internal/money"
)

// D adalah data view; field umum (User, Flash, dll) ditambahkan otomatis saat render.
type D map[string]any

// templates menyimpan satu *template.Template per halaman: layout + partials + file halaman.
// Di-parse sekali saat start sehingga render tidak mengalokasi parser per request.
type templates map[string]*template.Template

func parseTemplates(fsys fs.FS) (templates, error) {
	shared := []string{"templates/layouts/*.html", "templates/partials/*.html"}
	pages, err := fs.Glob(fsys, "templates/pages/*/*.html")
	if err != nil {
		return nil, err
	}
	out := make(templates, len(pages))
	for _, p := range pages {
		// "templates/pages/customers/index.html" -> "customers/index"
		name := strings.TrimSuffix(strings.TrimPrefix(p, "templates/pages/"), ".html")
		t := template.New(path.Base(p)).Funcs(funcs)
		for _, g := range shared {
			if t, err = t.ParseFS(fsys, g); err != nil {
				return nil, err
			}
		}
		if t, err = t.ParseFS(fsys, p); err != nil {
			return nil, fmt.Errorf("parse %s: %w", p, err)
		}
		out[name] = t
	}
	return out, nil
}

// render mengeksekusi block tertentu dari template halaman.
// block "base" = halaman penuh, "print" = layout cetak, nama lain = partial htmx.
func (a *App) render(w http.ResponseWriter, r *http.Request, status int, page, block string, data D) {
	t, ok := a.tpl[page]
	if !ok {
		a.serverError(w, r, fmt.Errorf("template %q tidak ada", page))
		return
	}
	if data == nil {
		data = D{}
	}
	data["User"] = userFrom(r.Context())
	data["Path"] = r.URL.Path
	data["Company"] = a.cfg.CompanyName
	data["CompanyInfo"] = a.cfg.CompanyInfo
	if block == "base" || block == "print" {
		data["Flash"] = a.sessions.PopString(r.Context(), "flash")
	}

	// Render ke buffer dulu agar error template tidak menghasilkan respons setengah jadi.
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, block, data); err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func (a *App) page(w http.ResponseWriter, r *http.Request, page string, data D) {
	a.render(w, r, http.StatusOK, page, "base", data)
}

// redirect bekerja untuk request biasa maupun htmx (boosted).
func (a *App) redirect(w http.ResponseWriter, r *http.Request, to string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Location", fmt.Sprintf(`{"path":%q,"target":"body"}`, to))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func (a *App) flash(r *http.Request, msg string) {
	a.sessions.Put(r.Context(), "flash", msg)
}

func (a *App) serverError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("server error", "method", r.Method, "path", r.URL.Path, "err", err)
	http.Error(w, "Terjadi kesalahan pada server", http.StatusInternalServerError)
}

// ---------- Template funcs ----------

var bulan = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
var bulanPanjang = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

func fmtDate(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return fmt.Sprintf("%02d %s %d", t.Day(), bulan[t.Month()-1], t.Year())
}

func fmtQty(q float64) string {
	return strings.ReplaceAll(strconv.FormatFloat(q, 'f', -1, 64), ".", ",")
}

var funcs = template.FuncMap{
	"rp":   money.Format,
	"date": fmtDate,
	"dateLong": func(t time.Time) string {
		return fmt.Sprintf("%d %s %d", t.Day(), bulanPanjang[t.Month()-1], t.Year())
	},
	"datep": func(t *time.Time) string {
		if t == nil {
			return "-"
		}
		return fmtDate(*t)
	},
	"iso": func(t any) string {
		switch v := t.(type) {
		case time.Time:
			if v.IsZero() {
				return ""
			}
			return v.Format("2006-01-02")
		case *time.Time:
			if v == nil {
				return ""
			}
			return v.Format("2006-01-02")
		}
		return ""
	},
	"datetime": func(t time.Time) string { return fmtDate(t) + " " + t.Local().Format("15:04") },
	"month":    func(t time.Time) string { return bulanPanjang[t.Month()-1] + " " + strconv.Itoa(t.Year()) },
	"qty":      fmtQty,
	"add":      func(a, b int) int { return a + b },
	// pct = persentase v terhadap max (untuk lebar bar chart sederhana).
	"pct": func(v, max int64) int64 {
		if max <= 0 {
			return 0
		}
		return v * 100 / max
	},
	// dict membuat map untuk meneruskan beberapa nilai ke sub-template.
	"dict": func(kv ...any) map[string]any {
		m := make(map[string]any, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	},
	"pid": func(p *int64) int64 {
		if p == nil {
			return 0
		}
		return *p
	},
	"sub": func(a, b int64) int64 { return a - b },
	"deref": func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	},
	"statusClass": func(s string) string {
		switch s {
		case "lunas":
			return "bg-emerald-100 text-emerald-800"
		case "sebagian":
			return "bg-amber-100 text-amber-800"
		default:
			return "bg-rose-100 text-rose-800"
		}
	},
	"statusLabel": func(s string) string {
		switch s {
		case "lunas":
			return "Lunas"
		case "sebagian":
			return "Dibayar sebagian"
		default:
			return "Belum bayar"
		}
	},
	"methodLabel": func(s string) string {
		switch s {
		case "cash":
			return "Tunai"
		case "transfer":
			return "Transfer"
		default:
			return "Lainnya"
		}
	},
	// active menandai menu sidebar yang sedang dibuka.
	"active": func(cur, prefix string) bool {
		if prefix == "/" {
			return cur == "/"
		}
		return strings.HasPrefix(cur, prefix)
	},
	// pageURL mengganti parameter page pada query string saat ini.
	"pageURL": func(base string, q url.Values, page int) string {
		v := url.Values{}
		for k, vs := range q {
			v[k] = vs
		}
		v.Set("page", strconv.Itoa(page))
		return base + "?" + v.Encode()
	},
	"withQuery": func(base string, q url.Values, kv ...string) string {
		v := url.Values{}
		for k, vs := range q {
			v[k] = vs
		}
		for i := 0; i+1 < len(kv); i += 2 {
			v.Set(kv[i], kv[i+1])
		}
		return base + "?" + v.Encode()
	},
}
