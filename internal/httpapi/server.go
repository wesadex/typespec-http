// Package httpapi is the HTTP layer: request validation, the generated router
// and the handler that implements the API.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/wesadex/typespec-http/internal/api"
	"github.com/wesadex/typespec-http/internal/order"
)

// NewServer returns the HTTP handler of the Orders API. Every request is
// logged, then checked against the OpenAPI spec; invalid ones never reach the handler.
func NewServer(orders *order.Service, log *slog.Logger) (http.Handler, error) {
	spec, err := api.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load embedded OpenAPI spec: %w", err)
	}
	// Match requests on any host, not only on the servers listed in the spec.
	spec.Servers = nil

	strict := api.NewStrictHandlerWithOptions(&handler{orders: orders}, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		},
	})

	router := api.HandlerWithOptions(strict, api.StdHTTPServerOptions{
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		},
	})

	validate := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		ErrorHandlerWithOpts: validationError,
	})

	// The access log is the outermost layer, so it also sees validation errors.
	return accessLog(log, validate(router)), nil
}

func validationError(_ context.Context, err error, w http.ResponseWriter, _ *http.Request, opts nethttpmiddleware.ErrorHandlerOpts) {
	switch {
	case errors.Is(err, routers.ErrMethodNotAllowed):
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	case opts.MatchedRoute == nil:
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", requestErrorMessage(err))
	}
}

// requestErrorMessage keeps the reason and drops the schema dump of kin-openapi.
func requestErrorMessage(err error) string {
	if reqErr, ok := errors.AsType[*openapi3filter.RequestError](err); ok && reqErr.Parameter != nil {
		reason := reqErr.Reason
		if schemaErr, ok := errors.AsType[*openapi3.SchemaError](reqErr.Err); ok {
			reason = schemaErr.Reason
		} else if reason == "" && reqErr.Err != nil {
			reason = reqErr.Err.Error()
		}
		return fmt.Sprintf("parameter %q in %s: %s", reqErr.Parameter.Name, reqErr.Parameter.In, reason)
	}
	return err.Error()
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorBody{Code: code, Message: message})
}
