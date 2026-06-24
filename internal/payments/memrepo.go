package payments

import (
	"context"
	"sync"
)

// MemRepository is an in-memory Repository used by unit and handler tests and
// for running the service without a database. It is safe for concurrent use.
type MemRepository struct {
	mu      sync.RWMutex
	store   map[string]DomesticPayment // by DomesticPaymentID
	byIdemp map[string]string          // idempotency key -> DomesticPaymentID
}

// NewMemRepository returns an empty in-memory repository.
func NewMemRepository() *MemRepository {
	return &MemRepository{
		store:   make(map[string]DomesticPayment),
		byIdemp: make(map[string]string),
	}
}

func (r *MemRepository) Create(_ context.Context, p *DomesticPayment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[p.DomesticPaymentID] = *p // store a copy so external mutation cannot corrupt state
	if p.IdempotencyKey != "" {
		r.byIdemp[p.IdempotencyKey] = p.DomesticPaymentID
	}
	return nil
}

func (r *MemRepository) Get(_ context.Context, id string) (*DomesticPayment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.store[id]
	if !ok {
		return nil, ErrNotFound
	}
	out := p
	return &out, nil
}

func (r *MemRepository) GetByIdempotencyKey(_ context.Context, key string) (*DomesticPayment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byIdemp[key]
	if !ok {
		return nil, ErrNotFound
	}
	p := r.store[id]
	out := p
	return &out, nil
}
