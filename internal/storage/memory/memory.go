// Package memory is an in-memory order repository, for local runs and tests.
package memory

import (
	"context"
	"sync"

	"github.com/wesadex/typespec-http/internal/order"
)

var _ order.Repository = (*Repository)(nil)

type Repository struct {
	mu     sync.RWMutex
	orders map[int64]order.Order
}

func New(orders ...order.Order) *Repository {
	r := &Repository{orders: make(map[int64]order.Order, len(orders))}
	for _, o := range orders {
		r.orders[o.ID] = o
	}
	return r
}

func (r *Repository) GetByID(_ context.Context, id int64) (order.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	o, ok := r.orders[id]
	if !ok {
		return order.Order{}, order.ErrNotFound
	}
	return o, nil
}
