package payments

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/sreeni/openbank-bian/pkg/consentcli"
	"github.com/sreeni/openbank-bian/pkg/httpx"
	"github.com/sreeni/openbank-bian/pkg/obie"
)

// ConsentClient is the slice of the consent service this domain depends on:
// fetch a consent view and mark a single-use payment consent as consumed.
// *consentcli.Client satisfies it; tests inject a fake so the consent service
// need not be running.
type ConsentClient interface {
	Get(ctx context.Context, id string) (*consentcli.View, error)
	Consume(ctx context.Context, id string) error
}

// Service holds the payment business logic. The consent client, clock and id
// generator are injected so tests are deterministic and the consent service can
// be faked.
type Service struct {
	repo    Repository
	consent ConsentClient
	now     func() time.Time
	newID   func() string
}

// NewService wires a Service to its repository and consent client using a real
// clock and UUID ids.
func NewService(repo Repository, consent ConsentClient) *Service {
	return &Service{
		repo:    repo,
		consent: consent,
		now:     time.Now,
		newID:   uuid.NewString,
	}
}

// CreateInput carries everything needed to accept a domestic payment, kept
// independent of the OBIE wire shapes. IdempotencyKey is the mandatory
// x-idempotency-key header.
type CreateInput struct {
	IdempotencyKey            string
	ConsentID                 string
	InstructionIdentification string
	EndToEndIdentification    string
	InstructedAmount          obie.Amount
	CreditorAccount           Account
	DebtorAccount             *Account
	Reference                 string
}

// Create accepts a domestic payment against an authorised, single-use consent.
//
// The flow is, in order:
//  1. Require the x-idempotency-key header.
//  2. Replay: if a payment already exists for that key, return it unchanged
//     without re-consuming the consent.
//  3. Validate the consent: it must exist, be of type domestic-payment, be
//     Authorised (a Consumed/Revoked consent is rejected) and its
//     InstructedAmount must match the request exactly.
//  4. Consume the consent first, then persist the payment — so a failed consume
//     never leaves an orphaned accepted payment behind.
func (s *Service) Create(ctx context.Context, in CreateInput) (*DomesticPayment, error) {
	if in.IdempotencyKey == "" {
		return nil, httpx.BadRequest("x-idempotency-key header is required",
			httpx.Detail(obie.ErrHeaderMissing, "missing x-idempotency-key", ""))
	}

	// Idempotent replay: a retry with the same key returns the original payment
	// and must not create a duplicate or re-consume the consent.
	if existing, err := s.repo.GetByIdempotencyKey(ctx, in.IdempotencyKey); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, httpx.Internal("could not check idempotency key")
	}

	if in.ConsentID == "" {
		return nil, httpx.BadRequest("ConsentId is required",
			httpx.Detail(obie.ErrFieldMissing, "missing ConsentId", "Data.ConsentId"))
	}

	view, err := s.consent.Get(ctx, in.ConsentID)
	if err != nil {
		if errors.Is(err, consentcli.ErrNotFound) {
			return nil, httpx.BadRequest("Consent not found",
				httpx.Detail(obie.ErrResourceNotFound, "no such consent", "Data.ConsentId"))
		}
		return nil, httpx.Internal("could not load consent")
	}
	if view.Type != consentcli.TypeDomesticPayment {
		return nil, httpx.Forbidden("Consent is not a domestic-payment consent",
			httpx.Detail(obie.ErrResourceInvalid, "unexpected consent type: "+view.Type, "Data.ConsentId"))
	}
	if view.Status != consentcli.StatusAuthorised {
		return nil, httpx.Forbidden("Consent is not authorised",
			httpx.Detail(obie.ErrResourceInvalid, "consent status is "+view.Status, "Data.ConsentId"))
	}

	// The submitted instruction must match the consent the PSU authorised: the
	// amount value AND currency must be identical.
	if view.InstructedAmount == nil || !amountsEqual(*view.InstructedAmount, in.InstructedAmount) {
		return nil, httpx.BadRequest("InstructedAmount does not match the consent",
			httpx.Detail(obie.ErrResourceConsentMismatch,
				"the instructed amount must match the authorised consent",
				"Data.Initiation.InstructedAmount"))
	}

	// Consume the consent before persisting the payment. A single-use consent
	// can only be consumed once; if this fails we have not yet created a
	// payment, so there is no orphan to clean up.
	if err := s.consent.Consume(ctx, in.ConsentID); err != nil {
		if errors.Is(err, consentcli.ErrNotFound) {
			return nil, httpx.BadRequest("Consent not found",
				httpx.Detail(obie.ErrResourceNotFound, "no such consent", "Data.ConsentId"))
		}
		// A non-2xx (e.g. already-consumed) consume is a conflict: the consent
		// has been used by another payment.
		return nil, httpx.Conflict("Consent could not be consumed",
			httpx.Detail(obie.ErrResourceInvalid, "consent is no longer usable", "Data.ConsentId"))
	}

	now := s.now()
	p := &DomesticPayment{
		DomesticPaymentID:         s.newID(),
		ConsentID:                 in.ConsentID,
		Status:                    StatusAcceptedSettlementInProcess,
		CreationDateTime:          now,
		StatusUpdateDateTime:      now,
		IdempotencyKey:            in.IdempotencyKey,
		InstructionIdentification: in.InstructionIdentification,
		EndToEndIdentification:    in.EndToEndIdentification,
		InstructedAmount:          in.InstructedAmount,
		CreditorAccount:           in.CreditorAccount,
		DebtorAccount:             in.DebtorAccount,
		Reference:                 in.Reference,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		// The consent has been consumed but we failed to persist the payment.
		// This is an internal failure; the consumed consent is the safe side to
		// err on (no money moves without a recorded payment).
		return nil, httpx.Internal("could not persist payment")
	}
	return p, nil
}

// Get returns an accepted payment by id, mapping a missing payment to a 404.
func (s *Service) Get(ctx context.Context, id string) (*DomesticPayment, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, s.mapNotFound(err)
	}
	return p, nil
}

func (s *Service) mapNotFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("Payment not found",
			httpx.Detail(obie.ErrResourceNotFound, "no such payment", ""))
	}
	return httpx.Internal("could not load payment")
}

// amountsEqual reports whether two OBIE amounts have the same value and
// currency. It compares the decimal values exactly (165.88 == 165.880).
func amountsEqual(a, b obie.Amount) bool {
	return a.Currency == b.Currency && a.Value.Equal(b.Value)
}
