package memory_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/wesadex/typespec-http/internal/order"
	"github.com/wesadex/typespec-http/internal/storage/memory"
)

func TestGetByID(t *testing.T) {
	repo := memory.New(
		order.Order{ID: 1, UserID: 10},
		order.Order{ID: 2, UserID: 20},
	)

	got, err := repo.GetByID(t.Context(), 2)
	if err != nil {
		t.Fatalf("GetByID(2) error = %v", err)
	}
	if got.ID != 2 || got.UserID != 20 {
		t.Errorf("GetByID(2) = %+v, want ID 2, UserID 20", got)
	}

	if _, err := repo.GetByID(t.Context(), 3); !errors.Is(err, order.ErrNotFound) {
		t.Errorf("GetByID(3) error = %v, want ErrNotFound", err)
	}
}

func TestNewSameIDLastWins(t *testing.T) {
	repo := memory.New(
		order.Order{ID: 1, UserID: 10},
		order.Order{ID: 1, UserID: 11},
	)

	got, err := repo.GetByID(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != 11 {
		t.Errorf("UserID = %d, want 11 (the last order with this id)", got.UserID)
	}
}

// Run with -race: parallel reads must be safe.
func TestGetByIDConcurrent(t *testing.T) {
	repo := memory.New(order.Order{ID: 1})

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if _, err := repo.GetByID(t.Context(), 1); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}
