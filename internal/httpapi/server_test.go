package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
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

type panickingRepo struct{}

func (panickingRepo) GetByID(context.Context, int64) (order.Order, error) {
	panic("bug in storage")
}

func TestGetOrderByID(t *testing.T) {
	tests := []struct {
		name      string
		repo      order.Repository
		method    string
		path      string
		wantCode  int
		wantErr   string // ErrorBody.Code, empty for success
		wantAllow string // Allow header of a 405 response
		notInSpec bool   // the response cannot be in the spec: no such operation
	}{
		{name: "found", method: http.MethodGet, path: "/orders/1", wantCode: http.StatusOK},
		{name: "not found", method: http.MethodGet, path: "/orders/2", wantCode: http.StatusNotFound, wantErr: "order_not_found"},
		{name: "id below minimum", method: http.MethodGet, path: "/orders/0", wantCode: http.StatusBadRequest, wantErr: "invalid_request"},
		{name: "negative id", method: http.MethodGet, path: "/orders/-1", wantCode: http.StatusBadRequest, wantErr: "invalid_request"},
		{name: "id above int64", method: http.MethodGet, path: "/orders/99999999999999999999", wantCode: http.StatusBadRequest, wantErr: "invalid_request"},
		{name: "id not a number", method: http.MethodGet, path: "/orders/abc", wantCode: http.StatusBadRequest, wantErr: "invalid_request"},
		{name: "unknown path", method: http.MethodGet, path: "/customers/1", wantCode: http.StatusNotFound, wantErr: "not_found", notInSpec: true},
		{name: "no id", method: http.MethodGet, path: "/orders/", wantCode: http.StatusNotFound, wantErr: "not_found", notInSpec: true},
		{name: "trailing slash", method: http.MethodGet, path: "/orders/1/", wantCode: http.StatusNotFound, wantErr: "not_found", notInSpec: true},
		{name: "wrong method", method: http.MethodDelete, path: "/orders/1", wantCode: http.StatusMethodNotAllowed,
			wantErr: "method_not_allowed", wantAllow: "GET", notInSpec: true},
		{name: "head", method: http.MethodHead, path: "/orders/1", wantCode: http.StatusMethodNotAllowed,
			wantErr: "method_not_allowed", wantAllow: "GET", notInSpec: true},
		{name: "storage error", repo: failingRepo{}, method: http.MethodGet, path: "/orders/1", wantCode: http.StatusInternalServerError, wantErr: "internal_error"},
		{name: "panic", repo: panickingRepo{}, method: http.MethodGet, path: "/orders/1", wantCode: http.StatusInternalServerError, wantErr: "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.repo
			if repo == nil {
				repo = memory.New(testOrder)
			}
			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil)
			rec := serve(t, repo, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantCode, rec.Body)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			if got := rec.Header().Get("Allow"); got != tt.wantAllow {
				t.Errorf("Allow = %q, want %q", got, tt.wantAllow)
			}
			if !tt.notInSpec {
				assertMatchesSpec(t, req, rec)
			}

			if tt.wantErr != "" {
				var body api.ErrorBody
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode error body: %v", err)
				}
				if body.Code != tt.wantErr || body.Message == "" || strings.HasSuffix(body.Message, ": ") {
					t.Errorf("error body = %+v, want code %q and a full message", body, tt.wantErr)
				}
				if strings.Contains(body.Message, "database") || strings.Contains(body.Message, "storage") {
					t.Errorf("error message shows internal details: %q", body.Message)
				}
				return
			}

			var got api.Order
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode order: %v", err)
			}
			want := api.Order{
				Id:              1,
				UserId:          42,
				CreatedDate:     testOrder.CreatedDate,
				LastChangedDate: testOrder.LastChangedDate,
				LastChangedBy:   7,
				TotalAmount:     api.Money{Amount: "1999.90", Currency: "EUR"},
				DeliveryAddress: "Alexanderplatz 1, 10178 Berlin",
				Status:          api.OrderStatusPaid,
			}
			if got != want {
				t.Errorf("order = %+v\nwant    %+v", got, want)
			}
		})
	}
}

// allStatuses lists every order.Status. Keep it in sync with the order package.
var allStatuses = []order.Status{order.StatusCreated, order.StatusPaid, order.StatusCancelled, order.StatusDone}

// TestOrderMapping checks how domain values become API values: the response
// matches the spec, or the server returns 500. It never changes money.
func TestOrderMapping(t *testing.T) {
	berlin := time.FixedZone("CET", 3600)
	tests := []struct {
		name       string
		change     func(o *order.Order)
		wantCode   int
		wantAmount string   // total_amount.amount of a 200 response
		wantJSON   []string // parts of the raw response body
	}{
		{name: "one decimal is padded", change: func(*order.Order) {}, wantCode: 200, wantAmount: "1999.90"},
		{name: "zero", change: amount("0"), wantCode: 200, wantAmount: "0.00"},
		{name: "negative", change: amount("-5.5"), wantCode: 200, wantAmount: "-5.50"},
		{name: "large", change: amount("123456789012345678901234567890.12"), wantCode: 200,
			wantAmount: "123456789012345678901234567890.12"},
		{name: "trailing zeros", change: amount("1.000"), wantCode: 200, wantAmount: "1.00"},
		{name: "three decimals are not rounded", change: amount("1.005"), wantCode: 500},
		{name: "lowercase currency", change: func(o *order.Order) { o.TotalAmount.Currency = "eur" }, wantCode: 500},
		{name: "long currency", change: func(o *order.Order) { o.TotalAmount.Currency = "EURO" }, wantCode: 500},
		{name: "unknown status", change: func(o *order.Order) { o.Status = "lost" }, wantCode: 500},
		{name: "times are sent in UTC", change: func(o *order.Order) {
			o.CreatedDate = time.Date(2026, 10, 30, 10, 15, 0, 0, berlin)
			o.LastChangedDate = time.Date(2026, 10, 30, 12, 15, 0, 0, berlin)
		}, wantCode: 200, wantAmount: "1999.90",
			wantJSON: []string{`"created_date":"2026-10-30T09:15:00Z"`, `"last_changed_date":"2026-10-30T11:15:00Z"`}},
	}
	for _, s := range allStatuses {
		tests = append(tests, struct {
			name       string
			change     func(o *order.Order)
			wantCode   int
			wantAmount string
			wantJSON   []string
		}{name: "status " + string(s), change: func(o *order.Order) { o.Status = s }, wantCode: 200,
			wantAmount: "1999.90", wantJSON: []string{`"status":"` + string(s) + `"`}})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := testOrder
			tt.change(&o)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/orders/1", nil)
			rec := serve(t, memory.New(o), req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantCode, rec.Body)
			}
			assertMatchesSpec(t, req, rec)
			for _, part := range tt.wantJSON {
				if !strings.Contains(rec.Body.String(), part) {
					t.Errorf("body = %s, want it to contain %s", rec.Body, part)
				}
			}
			if tt.wantCode != http.StatusOK {
				return
			}
			var got api.Order
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode order: %v", err)
			}
			if got.TotalAmount.Amount != tt.wantAmount {
				t.Errorf("amount = %q, want %q", got.TotalAmount.Amount, tt.wantAmount)
			}
		})
	}
}

func amount(s string) func(o *order.Order) {
	return func(o *order.Order) { o.TotalAmount.Amount = decimal.RequireFromString(s) }
}

// Every status of the spec must exist in the domain; TestOrderMapping checks
// the other direction.
func TestSpecStatusesExistInDomain(t *testing.T) {
	spec, err := api.GetSpec()
	if err != nil {
		t.Fatal(err)
	}
	schema := spec.Components.Schemas["OrderStatus"]
	if schema == nil || len(schema.Value.Enum) == 0 {
		t.Fatal("spec has no OrderStatus enum")
	}
	for _, v := range schema.Value.Enum {
		if !slices.Contains(allStatuses, order.Status(fmt.Sprint(v))) {
			t.Errorf("spec status %q has no order.Status", v)
		}
	}
}

// FuzzOrderAmount: any amount and currency give a response that matches the
// spec with the exact amount, or a 500.
func FuzzOrderAmount(f *testing.F) {
	for _, seed := range []struct{ amount, currency string }{
		{"1999.9", "EUR"}, {"0", "USD"}, {"-0.01", "JPY"}, {"1.005", "EUR"}, {"1e3", "EUR"},
		{"0.1234567890123456789", "EUR"}, {"5", "eur"}, {"5", ""}, {"5", "ÄÖÜ"},
	} {
		f.Add(seed.amount, seed.currency)
	}
	f.Fuzz(func(t *testing.T, amount, currency string) {
		d, err := decimal.NewFromString(amount)
		if err != nil {
			t.Skip()
		}
		o := testOrder
		o.TotalAmount = order.Money{Amount: d, Currency: currency}
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/orders/1", nil)
		rec := serve(t, memory.New(o), req)

		switch rec.Code {
		case http.StatusOK:
			assertMatchesSpec(t, req, rec)
			var got api.Order
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode order: %v", err)
			}
			if sent := decimal.RequireFromString(got.TotalAmount.Amount); !sent.Equal(d) {
				t.Errorf("amount %s was sent as %s", d, got.TotalAmount.Amount)
			}
		case http.StatusInternalServerError:
			assertMatchesSpec(t, req, rec)
		default:
			t.Errorf("status = %d, want 200 or 500; body: %s", rec.Code, rec.Body)
		}
	})
}

type ctxKey struct{}

type ctxRepo struct{ got any }

func (r *ctxRepo) GetByID(ctx context.Context, _ int64) (order.Order, error) {
	r.got = ctx.Value(ctxKey{})
	return testOrder, nil
}

func TestContextReachesRepository(t *testing.T) {
	repo := &ctxRepo{}
	ctx := context.WithValue(t.Context(), ctxKey{}, "request-ctx")
	rec := serve(t, repo, httptest.NewRequestWithContext(ctx, http.MethodGet, "/orders/1", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	if repo.got != "request-ctx" {
		t.Errorf("repository got context value %v, want the request context", repo.got)
	}
}

func TestPanicIsLogged(t *testing.T) {
	var buf bytes.Buffer
	srv, err := NewServer(order.NewService(panickingRepo{}), slog.New(slog.NewJSONHandler(&buf, nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/orders/1", nil))

	line := findLogLine(t, buf.String(), "panic in handler")
	stack, _ := line["stack"].(string)
	if line["level"] != "ERROR" || line["panic"] != "bug in storage" || !strings.Contains(stack, "panickingRepo") {
		t.Errorf("panic log = %v, want level ERROR, the panic value and the stack", line)
	}
}

// http.ErrAbortHandler must reach net/http, which uses it to abort a response.
func TestRecoverPanicKeepsAbortHandler(t *testing.T) {
	h := recoverPanic(slog.New(slog.DiscardHandler), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		p := recover()
		if err, ok := p.(error); !ok || !errors.Is(err, http.ErrAbortHandler) {
			t.Errorf("recovered %v, want http.ErrAbortHandler", p)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	t.Error("the panic was swallowed")
}

// The validator runs first, so in the full stack requestError is not reached.
func TestRequestError(t *testing.T) {
	rec := httptest.NewRecorder()
	requestError(rec, nil, errors.New("bad body"))

	var body api.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest || body != (api.ErrorBody{Code: "invalid_request", Message: "bad body"}) {
		t.Errorf("got %d %+v, want 400 invalid_request / bad body", rec.Code, body)
	}
}

// serve sends req through the full HTTP stack with repo behind it.
func serve(t *testing.T, repo order.Repository, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	srv, err := NewServer(order.NewService(repo), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
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
