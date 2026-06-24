package payments

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/sreeni/openbank-bian/pkg/consentcli"
	"github.com/sreeni/openbank-bian/pkg/httpx"
	"github.com/sreeni/openbank-bian/pkg/obie"
)

// fakeConsent is a test double for the consent service. It returns a fixed view
// and records how many times Consume is called, so tests can assert single-use
// semantics without running the real consent service.
type fakeConsent struct {
	view         *consentcli.View
	getErr       error
	consumeErr   error
	consumeCalls int
}

func (f *fakeConsent) Get(_ context.Context, _ string) (*consentcli.View, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.view, nil
}

func (f *fakeConsent) Consume(_ context.Context, _ string) error {
	f.consumeCalls++
	return f.consumeErr
}

// authorisedView builds a domestic-payment consent view in Authorised status
// with the given amount, ready to back a successful payment.
func authorisedView(amount, currency string) *consentcli.View {
	amt := obie.MustAmount(amount, currency)
	return &consentcli.View{
		ConsentID:        "cons-1",
		Type:             consentcli.TypeDomesticPayment,
		Status:           consentcli.StatusAuthorised,
		InstructedAmount: &amt,
	}
}

// newTestService returns a service backed by an in-memory repo and the given
// fake consent client, with a fixed clock and deterministic ids.
func newTestService(fc *fakeConsent) *Service {
	s := NewService(NewMemRepository(), fc)
	fixed := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return fixed }
	n := 0
	s.newID = func() string { n++; return "pay-" + itoa(n) }
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func wantStatus(t *testing.T, err error, status int) {
	t.Helper()
	var apiErr *httpx.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *httpx.APIError, got %v", err)
	}
	if apiErr.Status != status {
		t.Fatalf("status = %d, want %d (%s)", apiErr.Status, status, apiErr.Message)
	}
}

// validInput is a baseline CreateInput that pairs with authorisedView.
func validInput() CreateInput {
	return CreateInput{
		IdempotencyKey:            "idem-1",
		ConsentID:                 "cons-1",
		InstructionIdentification: "ID-1",
		EndToEndIdentification:    "E2E-1",
		InstructedAmount:          obie.MustAmount("165.88", "GBP"),
		CreditorAccount:           Account{SchemeName: "UK.OBIE.SortCodeAccountNumber", Identification: "08080021325698", Name: "ACME"},
		Reference:                 "INV-9",
	}
}

func TestCreateHappyPath(t *testing.T) {
	ctx := context.Background()
	fc := &fakeConsent{view: authorisedView("165.88", "GBP")}
	s := newTestService(fc)

	p, err := s.Create(ctx, validInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Status != StatusAcceptedSettlementInProcess {
		t.Fatalf("status = %s", p.Status)
	}
	if p.DomesticPaymentID != "pay-1" || p.ConsentID != "cons-1" {
		t.Fatalf("unexpected payment %+v", p)
	}
	if fc.consumeCalls != 1 {
		t.Fatalf("Consume called %d times, want 1", fc.consumeCalls)
	}

	// The payment is retrievable by id.
	got, err := s.Get(ctx, p.DomesticPaymentID)
	if err != nil || got.DomesticPaymentID != p.DomesticPaymentID {
		t.Fatalf("get: err=%v got=%+v", err, got)
	}
}

func TestCreateRequiresIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	fc := &fakeConsent{view: authorisedView("165.88", "GBP")}
	s := newTestService(fc)

	in := validInput()
	in.IdempotencyKey = ""
	_, err := s.Create(ctx, in)
	wantStatus(t, err, http.StatusBadRequest)
	if fc.consumeCalls != 0 {
		t.Fatalf("Consume should not be called when idempotency key is missing")
	}
}

func TestCreateIdempotentReplay(t *testing.T) {
	ctx := context.Background()
	fc := &fakeConsent{view: authorisedView("165.88", "GBP")}
	s := newTestService(fc)

	first, err := s.Create(ctx, validInput())
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	// Same idempotency key → return the original payment, no second consume.
	second, err := s.Create(ctx, validInput())
	if err != nil {
		t.Fatalf("replay create: %v", err)
	}
	if second.DomesticPaymentID != first.DomesticPaymentID {
		t.Fatalf("replay produced a new payment: %s != %s", second.DomesticPaymentID, first.DomesticPaymentID)
	}
	if fc.consumeCalls != 1 {
		t.Fatalf("Consume called %d times across replay, want 1", fc.consumeCalls)
	}
}

func TestCreateRequiresConsentID(t *testing.T) {
	ctx := context.Background()
	fc := &fakeConsent{view: authorisedView("165.88", "GBP")}
	s := newTestService(fc)

	in := validInput()
	in.ConsentID = ""
	_, err := s.Create(ctx, in)
	wantStatus(t, err, http.StatusBadRequest)
	if fc.consumeCalls != 0 {
		t.Fatal("Consume should not be called without a consent id")
	}
}

func TestCreateRejectsUnauthorisedConsent(t *testing.T) {
	ctx := context.Background()
	view := authorisedView("165.88", "GBP")
	view.Status = consentcli.StatusConsumed // already used
	fc := &fakeConsent{view: view}
	s := newTestService(fc)

	_, err := s.Create(ctx, validInput())
	wantStatus(t, err, http.StatusForbidden)
	if fc.consumeCalls != 0 {
		t.Fatal("Consume should not be called for a consumed consent")
	}
}

func TestCreateRejectsWrongConsentType(t *testing.T) {
	ctx := context.Background()
	view := authorisedView("165.88", "GBP")
	view.Type = consentcli.TypeAccountAccess
	fc := &fakeConsent{view: view}
	s := newTestService(fc)

	_, err := s.Create(ctx, validInput())
	wantStatus(t, err, http.StatusForbidden)
}

func TestCreateRejectsAmountMismatch(t *testing.T) {
	ctx := context.Background()
	fc := &fakeConsent{view: authorisedView("100.00", "GBP")} // consent says 100.00
	s := newTestService(fc)

	in := validInput() // request says 165.88
	_, err := s.Create(ctx, in)
	wantStatus(t, err, http.StatusBadRequest)

	var apiErr *httpx.APIError
	errors.As(err, &apiErr)
	if len(apiErr.Details) == 0 ||
		apiErr.Details[0].ErrorCode != obie.ErrResourceConsentMismatch ||
		apiErr.Details[0].Path != "Data.Initiation.InstructedAmount" {
		t.Fatalf("unexpected error detail %+v", apiErr.Details)
	}
	if fc.consumeCalls != 0 {
		t.Fatal("Consume should not be called on amount mismatch")
	}
}

func TestCreateRejectsCurrencyMismatch(t *testing.T) {
	ctx := context.Background()
	fc := &fakeConsent{view: authorisedView("165.88", "EUR")} // same value, different currency
	s := newTestService(fc)

	_, err := s.Create(ctx, validInput())
	wantStatus(t, err, http.StatusBadRequest)
}

func TestCreateUnknownConsentIsBadRequest(t *testing.T) {
	ctx := context.Background()
	fc := &fakeConsent{getErr: consentcli.ErrNotFound}
	s := newTestService(fc)

	_, err := s.Create(ctx, validInput())
	wantStatus(t, err, http.StatusBadRequest)
}

func TestCreateConflictWhenConsumeFails(t *testing.T) {
	ctx := context.Background()
	// Consent looks authorised, but the consume races and fails (e.g. used by a
	// concurrent payment) → conflict, and no payment is persisted.
	fc := &fakeConsent{
		view:       authorisedView("165.88", "GBP"),
		consumeErr: errors.New("consent service returned 409 consuming consent"),
	}
	s := newTestService(fc)

	_, err := s.Create(ctx, validInput())
	wantStatus(t, err, http.StatusConflict)

	// Nothing should have been persisted under the idempotency key.
	if _, err := s.repo.GetByIdempotencyKey(ctx, "idem-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("payment persisted despite failed consume: %v", err)
	}
}

func TestGetUnknownIsNotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestService(&fakeConsent{})
	_, err := s.Get(ctx, "nope")
	wantStatus(t, err, http.StatusNotFound)
}
