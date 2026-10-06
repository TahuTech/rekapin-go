// Package config memuat konfigurasi aplikasi dari environment variable.
package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	Addr         string // alamat listen HTTP, mis. "127.0.0.1:8080"
	DatabaseURL  string // DSN PostgreSQL
	SecureCookie bool   // true jika di belakang HTTPS (produksi)
	DBMaxConns   int32
	CompanyName  string // ditampilkan di header invoice
	CompanyInfo  string // alamat/kontak di invoice
}

func Load() (Config, error) {
	c := Config{
		Addr:         getenv("ADDR", "127.0.0.1:8080"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		SecureCookie: getenv("SECURE_COOKIE", "false") == "true",
		CompanyName:  getenv("COMPANY_NAME", "Rekapin"),
		CompanyInfo:  getenv("COMPANY_INFO", ""),
		DBMaxConns:   5,
	}
	if v := os.Getenv("DB_MAX_CONNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return c, errors.New("DB_MAX_CONNS harus bilangan positif")
		}
		c.DBMaxConns = int32(n)
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL wajib diisi")
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
