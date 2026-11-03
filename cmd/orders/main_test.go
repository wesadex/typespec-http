package main

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"

	"github.com/wesadex/typespec-http/internal/api"
	"github.com/wesadex/typespec-http/internal/httpapi"
	"github.com/wesadex/typespec-http/internal/order"
	"github.com/wesadex/typespec-http/internal/storage/memory"
)

// Every demo order goes through the real HTTP stack and must come back as a
// 200 response that matches the OpenAPI spec.
func TestDemoOrdersMatchSpec(t *testing.T) {
	orders := demoOrders()
	if len(orders) == 0 {
		t.Fatal("no demo orders")
	}
	srv, err := httpapi.NewServer(order.NewService(memory.New(orders...)), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := api.GetSpec()
	if err != nil {
		t.Fatal(err)
	}
	spec.Servers = nil
	router, err := gorillamux.NewRouter(spec)
	if err != nil {
		t.Fatal(err)
	}

	for _, o := range orders {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("/orders/%d", o.ID), nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("order %d: status = %d, want 200; body: %s", o.ID, rec.Code, rec.Body)
			continue
		}
		route, pathParams, err := router.FindRoute(req)
		if err != nil {
			t.Fatal(err)
		}
		err = openapi3filter.ValidateResponse(t.Context(), &openapi3filter.ResponseValidationInput{
			RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: pathParams, Route: route},
			Status:                 rec.Code,
			Header:                 rec.Header(),
			Body:                   io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
		})
		if err != nil {
			t.Errorf("order %d does not match the spec: %v", o.ID, err)
		}
	}
}
