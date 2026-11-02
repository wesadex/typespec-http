// Command orders runs the Orders API server.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shopspring/decimal"

	"github.com/wesadex/typespec-http/internal/httpapi"
	"github.com/wesadex/typespec-http/internal/order"
	"github.com/wesadex/typespec-http/internal/storage/memory"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	addr := flag.String("addr", envOr("ADDR", ":8080"), "listen address (env ADDR)")
	flag.Parse()

	orders := order.NewService(memory.New(demoOrders()...))
	handler, err := httpapi.NewServer(orders, log)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", *addr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// demoOrders fills the in-memory store until there is a real database.
func demoOrders() []order.Order {
	created := time.Date(2026, 10, 30, 9, 15, 0, 0, time.UTC)
	return []order.Order{{
		ID:              1,
		UserID:          42,
		CreatedDate:     created,
		LastChangedDate: created.Add(2 * time.Hour),
		LastChangedBy:   42,
		TotalAmount:     order.Money{Amount: decimal.RequireFromString("1999.90"), Currency: "EUR"},
		DeliveryAddress: "Alexanderplatz 1, 10178 Berlin",
		Status:          order.StatusPaid,
	}}
}
