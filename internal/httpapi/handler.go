package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/wesadex/typespec-http/internal/api"
	"github.com/wesadex/typespec-http/internal/order"
)

// handler implements the generated API interface. It only maps between the
// domain and the API types; all logic is in the order package.
type handler struct {
	orders *order.Service
}

var _ api.StrictServerInterface = (*handler)(nil)

func (h *handler) GetOrderById(ctx context.Context, req api.GetOrderByIdRequestObject) (api.GetOrderByIdResponseObject, error) {
	o, err := h.orders.Get(ctx, req.Id)
	if errors.Is(err, order.ErrNotFound) {
		return api.GetOrderById404JSONResponse{
			Code:    "order_not_found",
			Message: fmt.Sprintf("order %d not found", req.Id),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get order %d: %w", req.Id, err)
	}
	out, err := toAPIOrder(o)
	if err != nil {
		return nil, fmt.Errorf("order %d: %w", req.Id, err)
	}
	return api.GetOrderById200JSONResponse(out), nil
}

// toAPIOrder fails when the order cannot be shown as the spec says. A 500 is
// better than a response that breaks the contract, or money that is rounded
// without anyone noticing.
func toAPIOrder(o order.Order) (api.Order, error) {
	if !o.TotalAmount.Amount.Equal(o.TotalAmount.Amount.Round(2)) {
		return api.Order{}, fmt.Errorf("total amount %s has more than 2 decimal places", o.TotalAmount.Amount)
	}
	if !isCurrencyCode(o.TotalAmount.Currency) {
		return api.Order{}, fmt.Errorf("currency %q is not an ISO 4217 code", o.TotalAmount.Currency)
	}
	status := api.OrderStatus(o.Status)
	if !status.Valid() {
		return api.Order{}, fmt.Errorf("status %q is not in the API", o.Status)
	}
	return api.Order{
		Id:              o.ID,
		UserId:          o.UserID,
		CreatedDate:     o.CreatedDate.UTC(),
		LastChangedDate: o.LastChangedDate.UTC(),
		LastChangedBy:   o.LastChangedBy,
		TotalAmount: api.Money{
			Amount:   o.TotalAmount.Amount.StringFixed(2),
			Currency: o.TotalAmount.Currency,
		},
		DeliveryAddress: o.DeliveryAddress,
		Status:          status,
	}, nil
}

// isCurrencyCode reports whether s has the form of an ISO 4217 code: three
// capital letters.
func isCurrencyCode(s string) bool {
	if len(s) != 3 {
		return false
	}
	for _, c := range []byte(s) {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}
