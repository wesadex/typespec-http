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
	"runtime/debug"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
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

	specRouter, err := gorillamux.NewRouter(spec)
	if err != nil {
		return nil, fmt.Errorf("build router from OpenAPI spec: %w", err)
	}

	strict := api.NewStrictHandlerWithOptions(&handler{orders: orders}, nil, api.StrictHTTPServerOptions{
		// The validator checks requests first, so these two handlers only run
		// if the spec and the generated code disagree.
		RequestErrorHandlerFunc: requestError,
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		},
	})

	router := api.HandlerWithOptions(strict, api.StdHTTPServerOptions{
		ErrorHandlerFunc: requestError,
	})

	validate := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		ErrorHandlerWithOpts: func(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, opts nethttpmiddleware.ErrorHandlerOpts) {
			validationError(ctx, err, w, r, opts, specRouter)
		},
	})

	// The access log is the outermost layer, so it also sees validation errors
	// and panics.
	return accessLog(log, recoverPanic(log, validate(router))), nil
}

func requestError(w http.ResponseWriter, _ *http.Request, err error) {
	writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
}

func validationError(_ context.Context, err error, w http.ResponseWriter, r *http.Request, opts nethttpmiddleware.ErrorHandlerOpts, router routers.Router) {
	switch {
	case errors.Is(err, routers.ErrMethodNotAllowed):
		// RFC 9110: a 405 response lists the methods the resource supports.
		w.Header().Set("Allow", strings.Join(allowedMethods(router, r), ", "))
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	case opts.MatchedRoute == nil:
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", requestErrorMessage(err))
	}
}

// allowedMethods returns the methods of the spec that exist for the path of r.
func allowedMethods(router routers.Router, r *http.Request) []string {
	var allowed []string
	for _, method := range []string{
		http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions,
	} {
		probe := r.Clone(r.Context())
		probe.Method = method
		if _, _, err := router.FindRoute(probe); err == nil {
			allowed = append(allowed, method)
		}
	}
	return allowed
}

// recoverPanic turns a panic in a handler into a 500 response and a log line
// with the stack, instead of a dropped connection.
func recoverPanic(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			// net/http uses this panic to abort a response on purpose.
			if err, ok := p.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(p)
			}
			log.ErrorContext(r.Context(), "panic in handler",
				"method", r.Method, "path", r.URL.Path, "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}()
		next.ServeHTTP(w, r)
	})
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
