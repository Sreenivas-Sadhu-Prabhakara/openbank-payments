package payments

import (
	"context"
	"errors"
)

// ErrNotFound is returned by a Repository when no payment matches the lookup.
var ErrNotFound = errors.New("payment not found")

// Repository is the persistence port for domestic payments. Both the in-memory
// and the Postgres implementations satisfy it, so the same business-logic tests
// run against either store.
type Repository interface {
	Create(ctx context.Context, p *DomesticPayment) error
	Get(ctx context.Context, id string) (*DomesticPayment, error)
	// GetByIdempotencyKey returns the payment previously created with key, or
	// ErrNotFound if none — used to detect and replay idempotent retries.
	GetByIdempotencyKey(ctx context.Context, key string) (*DomesticPayment, error)
}
