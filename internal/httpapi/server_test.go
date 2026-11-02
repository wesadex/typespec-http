package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/shopspring/decimal"

	"github.com/wesadex/typespec-http/internal/api"
	"github.com/wesadex/typespec-http/internal/order"
	"github.com/wesadex/typespec-http/internal/storage/memory"
)

var testOrder = order.Order{
	ID:              1,
	UserID:          42,
	CreatedDate:     time.Date(2026, 10, 30, 9, 15, 0, 0, time.UTC),
	LastChangedDate: time.Date(2026, 10, 30, 11, 15, 0, 0, time.UTC),
	LastChangedBy:   7,
	TotalAmount:     order.Money{Amount: decimal.RequireFromString("1999.9"), Currency: "EUR"},
	DeliveryAddress: "Alexanderplatz 1, 10178 Berlin",
	Status:          order.StatusPaid,
}

type failingRepo struct{}

func (failingRepo) GetByID(context.Context, int64) (order.Order, error) {
	return order.Order{}, errors.New("database is down")
}

func TestGetOrderByID(t *testing.T) {
	tests := []struct {
		name     string
		repo     order.Repository
		method   string
		path     string
		wantCode int
		wantErr  string // ErrorBody.Code, empty for success
	}{
		{name: "found", method: http.MethodGet, path: "/orders/1", wantCode: http.StatusOK},
		{name: "not found", method: http.MethodGet, path: "/orders/2", wantCode: http.StatusNotFound, wantErr: "order_not_found"},
		{name: "id below minimum", method: http.MethodGet, path: "/orders/0", wantCode: http.StatusBadRequest, wantErr: "invalid_request"},
		{name: "id not a number", method: http.MethodGet, path: "/orders/abc", wantCode: http.StatusBadRequest, wantErr: "invalid_request"},
		{name: "unknown path", method: http.MethodGet, path: "/customers/1", wantCode: http.StatusNotFound, wantErr: "not_found"},
		{name: "wrong method", method: http.MethodDelete, path: "/orders/1", wantCode: http.StatusMethodNotAllowed, wantErr: "method_not_allowed"},
		{name: "storage error", repo: failingRepo{}, method: http.MethodGet, path: "/orders/1", wantCode: http.StatusInternalServerError, wantErr: "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.repo
			if repo == nil {
				repo = memory.New(testOrder)
			}
			srv, err := NewServer(order.NewService(repo), slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantCode, rec.Body)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}

			if tt.wantErr != "" {
				var body api.ErrorBody
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode error body: %v", err)
				}
				if body.Code != tt.wantErr || body.Message == "" || strings.HasSuffix(body.Message, ": ") {
					t.Errorf("error body = %+v, want code %q and a full message", body, tt.wantErr)
				}
				return
			}

			var got api.Order
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode order: %v", err)
			}
			if got.Id != 1 || got.UserId != 42 || got.LastChangedBy != 7 {
				t.Errorf("ids = %d/%d/%d, want 1/42/7", got.Id, got.UserId, got.LastChangedBy)
			}
			if got.TotalAmount != (api.Money{Amount: "1999.90", Currency: "EUR"}) {
				t.Errorf("total_amount = %+v, want 1999.90 EUR", got.TotalAmount)
			}
			if got.Status != api.OrderStatusPaid {
				t.Errorf("status = %q, want paid", got.Status)
			}
			if !got.CreatedDate.Equal(testOrder.CreatedDate) {
				t.Errorf("created_date = %v, want %v", got.CreatedDate, testOrder.CreatedDate)
			}

			// The response of a known endpoint must match the OpenAPI spec.
			assertMatchesSpec(t, req, rec)
		})
	}
}

func TestErrorResponsesMatchSpec(t *testing.T) {
	srv, err := NewServer(order.NewService(memory.New()), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/orders/2", "/orders/0", "/orders/abc"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		assertMatchesSpec(t, req, rec)
	}
}

func assertMatchesSpec(t *testing.T, req *http.Request, rec *httptest.ResponseRecorder) {
	t.Helper()

	spec, err := api.GetSpec()
	if err != nil {
		t.Fatal(err)
	}
	spec.Servers = nil
	router, err := gorillamux.NewRouter(spec)
	if err != nil {
		t.Fatal(err)
	}
	route, pathParams, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("find route for %s %s: %v", req.Method, req.URL.Path, err)
	}

	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: pathParams,
			Route:      route,
		},
		Status: rec.Code,
		Header: rec.Header(),
		Body:   io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
	}
	if err := openapi3filter.ValidateResponse(context.Background(), in); err != nil {
		t.Errorf("%s %s: response does not match the spec: %v", req.Method, req.URL.Path, err)
	}
}
