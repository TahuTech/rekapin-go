// Package money berisi helper format & parsing nominal rupiah (disimpan sebagai int64).
package money

import (
	"errors"
	"strconv"
	"strings"
)

// Format mengubah 1500000 menjadi "1.500.000".
func Format(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatInt(v, 10)
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s[i : i+3])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// Parse menerima "1.500.000", "1500000", atau "Rp 1.500.000" dan mengembalikan 1500000.
func Parse(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "Rp"), "rp")
	s = strings.NewReplacer(".", "", " ", "", ",00", "").Replace(s)
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errors.New("nominal tidak valid")
	}
	return v, nil
}

// ParseQty menerima "1,5" atau "1.5" sebagai 1.5.
func ParseQty(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 {
		return 0, errors.New("qty harus lebih dari 0")
	}
	return v, nil
}
