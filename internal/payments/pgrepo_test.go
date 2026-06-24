//go:build integration

package payments

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sreeni/openbank-bian/pkg/obie"
	"github.com/sreeni/openbank-bian/pkg/pg"
	"github.com/sreeni/openbank-bian/pkg/testutil"
)

// newPgRepo spins up a throwaway Postgres, applies migrations and returns a
// Postgres-backed repository. Migrations are read from the module's migrations
// directory relative to this test package.
func newPgRepo(t *testing.T) *PgRepository {
	t.Helper()
	ctx := context.Background()
	dsn := testutil.PostgresDSN(t)

	pool, err := pg.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pg.RunMigrations(ctx, pool, os.DirFS("../.."), "migrations", "payments"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewPgRepository(pool)
}

func TestPgRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newPgRepo(t)

	amt := obie.MustAmount("165.88", "GBP")
	p := &DomesticPayment{
		DomesticPaymentID:         "pay-1",
		ConsentID:                 "cons-1",
		Status:                    StatusAcceptedSettlementInProcess,
		CreationDateTime:          time.Now().UTC().Truncate(time.Second),
		StatusUpdateDateTime:      time.Now().UTC().Truncate(time.Second),
		IdempotencyKey:            "idem-1",
		InstructionIdentification: "ID412",
		EndToEndIdentification:    "E2E412",
		InstructedAmount:          amt,
		CreditorAccount:           Account{SchemeName: "UK.OBIE.SortCodeAccountNumber", Identification: "0808", Name: "ACME"},
		DebtorAccount:             &Account{SchemeName: "UK.OBIE.SortCodeAccountNumber", Identification: "1234", Name: "Payer"},
		Reference:                 "INV-9",
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Get by id.
	got, err := repo.Get(ctx, p.DomesticPaymentID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusAcceptedSettlementInProcess || got.ConsentID != "cons-1" {
		t.Fatalf("unexpected payment %+v", got)
	}
	if got.InstructedAmount.String() != "165.88" || got.InstructedAmount.Currency != "GBP" {
		t.Fatalf("amount = %v", got.InstructedAmount)
	}
	if got.CreditorAccount.Name != "ACME" || got.DebtorAccount == nil || got.DebtorAccount.Identification != "1234" {
		t.Fatalf("accounts = %+v / %+v", got.CreditorAccount, got.DebtorAccount)
	}
	if got.Reference != "INV-9" {
		t.Fatalf("reference = %s", got.Reference)
	}

	// Get by idempotency key (hit).
	byKey, err := repo.GetByIdempotencyKey(ctx, "idem-1")
	if err != nil {
		t.Fatalf("get by key: %v", err)
	}
	if byKey.DomesticPaymentID != p.DomesticPaymentID {
		t.Fatalf("byKey id = %s", byKey.DomesticPaymentID)
	}
}

func TestPgRepositoryGetMissing(t *testing.T) {
	repo := newPgRepo(t)
	if _, err := repo.Get(context.Background(), "nope"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestPgRepositoryGetByIdempotencyKeyMissing(t *testing.T) {
	repo := newPgRepo(t)
	if _, err := repo.GetByIdempotencyKey(context.Background(), "no-such-key"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
