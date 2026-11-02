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
	return api.GetOrderById200JSONResponse(toAPIOrder(o)), nil
}

func toAPIOrder(o order.Order) api.Order {
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
		Status:          api.OrderStatus(o.Status),
	}
}
