package order_test

import (
	"context"
	"errors"
	"testing"

	"github.com/wesadex/typespec-http/internal/order"
)

type stubRepo struct {
	orders map[int64]order.Order
	err    error
	calls  []int64
}

func (r *stubRepo) GetByID(_ context.Context, id int64) (order.Order, error) {
	r.calls = append(r.calls, id)
	if r.err != nil {
		return order.Order{}, r.err
	}
	o, ok := r.orders[id]
	if !ok {
		return order.Order{}, order.ErrNotFound
	}
	return o, nil
}

func TestServiceGet(t *testing.T) {
	want := order.Order{ID: 7, UserID: 42, Status: order.StatusPaid}
	repo := &stubRepo{orders: map[int64]order.Order{7: want}}
	svc := order.NewService(repo)

	got, err := svc.Get(t.Context(), 7)
	if err != nil {
		t.Fatalf("Get(7) error = %v", err)
	}
	if got.ID != want.ID || got.UserID != want.UserID || got.Status != want.Status {
		t.Errorf("Get(7) = %+v, want %+v", got, want)
	}
	if len(repo.calls) != 1 || repo.calls[0] != 7 {
		t.Errorf("repository calls = %v, want [7]", repo.calls)
	}
}

func TestServiceGetNotFound(t *testing.T) {
	svc := order.NewService(&stubRepo{})

	_, err := svc.Get(t.Context(), 1)
	if !errors.Is(err, order.ErrNotFound) {
		t.Errorf("Get(1) error = %v, want ErrNotFound", err)
	}
}

func TestServiceGetRepositoryError(t *testing.T) {
	dbErr := errors.New("database is down")
	svc := order.NewService(&stubRepo{err: dbErr})

	_, err := svc.Get(t.Context(), 1)
	if !errors.Is(err, dbErr) {
		t.Errorf("Get(1) error = %v, want the repository error", err)
	}
	if errors.Is(err, order.ErrNotFound) {
		t.Error("a repository error must not look like ErrNotFound")
	}
}
