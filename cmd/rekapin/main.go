// Command rekapin: server web rekap penjualan & piutang khusus admin.
//
//	rekapin serve                      jalankan server (migrasi otomatis)
//	rekapin migrate                    jalankan migrasi saja
//	rekapin user create <username> <nama>   buat/reset admin (password dibaca dari stdin)
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/term"

	"rekapin/internal/config"
	"rekapin/internal/db"
	"rekapin/internal/store"
	"rekapin/internal/web"
	assets "rekapin/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := run(os.Args[1:]); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch cmd {
	case "migrate":
		return db.Migrate(ctx, pool)
	case "user":
		if len(args) < 4 || args[1] != "create" {
			return errors.New("pemakaian: rekapin user create <username> <nama lengkap>")
		}
		return createUser(ctx, store.New(pool), args[2], strings.Join(args[3:], " "))
	case "serve":
		if err := db.Migrate(ctx, pool); err != nil {
			return fmt.Errorf("migrasi: %w", err)
		}
		return serve(ctx, cfg, pool)
	default:
		return fmt.Errorf("perintah tidak dikenal: %s", cmd)
	}
}

func serve(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) error {
	app, err := web.New(cfg, pool, assets.FS)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("server berjalan", "addr", cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	slog.Info("shutdown...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func createUser(ctx context.Context, st *store.Store, username, name string) error {
	pw, err := readPassword()
	if err != nil {
		return err
	}
	if len(pw) < 8 {
		return errors.New("password minimal 8 karakter")
	}
	hash, err := web.HashPassword(pw)
	if err != nil {
		return err
	}
	if _, err := st.CreateUser(ctx, username, name, hash); err != nil {
		return err
	}
	fmt.Printf("Admin %q tersimpan.\n", username)
	return nil
}

// readPassword membaca tanpa echo bila dari terminal, atau satu baris dari stdin (untuk script).
func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Print("Password: ")
		b, err := term.ReadPassword(fd)
		fmt.Println()
		return string(b), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
