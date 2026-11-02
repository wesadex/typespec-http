// Package order is the domain of the Orders service. It knows nothing about HTTP
// or storage details.
package order

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

type Status string

const (
	StatusCreated   Status = "created"
	StatusPaid      Status = "paid"
	StatusCancelled Status = "cancelled"
	StatusDone      Status = "done"
)

// Money is an exact amount in one currency. Never use float64 for money.
type Money struct {
	Amount   decimal.Decimal
	Currency string // ISO 4217, for example "EUR"
}

type Order struct {
	ID              int64
	UserID          int64
	CreatedDate     time.Time
	LastChangedDate time.Time
	LastChangedBy   int64 // user id
	TotalAmount     Money
	DeliveryAddress string
	Status          Status
}

// ErrNotFound is returned when there is no order with the given id.
var ErrNotFound = errors.New("order not found")

// Repository stores orders. Implementations live in internal/storage.
type Repository interface {
	GetByID(ctx context.Context, id int64) (Order, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Get returns the order with the given id, or ErrNotFound.
func (s *Service) Get(ctx context.Context, id int64) (Order, error) {
	return s.repo.GetByID(ctx, id)
}
