package payments

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sreeni/openbank-bian/pkg/obie"
)

// PgRepository is the Postgres-backed Repository. The payments service owns the
// "payments" schema; this type touches nothing outside it.
type PgRepository struct {
	pool *pgxpool.Pool
}

// NewPgRepository returns a Postgres repository over the given pool.
func NewPgRepository(pool *pgxpool.Pool) *PgRepository {
	return &PgRepository{pool: pool}
}

const paymentColumns = `id, consent_id, status, creation_dt, status_update_dt, idempotency_key,
	instruction_id, e2e_id, instructed_amount, instructed_currency,
	creditor_scheme, creditor_ident, creditor_name,
	debtor_scheme, debtor_ident, debtor_name, reference`

func (r *PgRepository) Create(ctx context.Context, p *DomesticPayment) error {
	amount := p.InstructedAmount.String()
	currency := p.InstructedAmount.Currency
	dScheme, dIdent, dName := accountCols(p.DebtorAccount)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO payments.domestic_payments (`+paymentColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		p.DomesticPaymentID, p.ConsentID, string(p.Status), p.CreationDateTime, p.StatusUpdateDateTime,
		nullable(p.IdempotencyKey),
		nullable(p.InstructionIdentification), nullable(p.EndToEndIdentification), amount, currency,
		p.CreditorAccount.SchemeName, p.CreditorAccount.Identification, ptrOrNil(p.CreditorAccount.Name),
		dScheme, dIdent, dName, nullable(p.Reference),
	)
	return err
}

func (r *PgRepository) Get(ctx context.Context, id string) (*DomesticPayment, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments.domestic_payments WHERE id = $1`, id)
	p, err := scanPayment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (r *PgRepository) GetByIdempotencyKey(ctx context.Context, key string) (*DomesticPayment, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+paymentColumns+` FROM payments.domestic_payments WHERE idempotency_key = $1`, key)
	p, err := scanPayment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// scanPayment reads a row in paymentColumns order into a DomesticPayment.
func scanPayment(row pgx.Row) (*DomesticPayment, error) {
	var (
		p                      DomesticPayment
		status                 string
		idempKey               *string
		instrID, e2eID         *string
		amount, currency       string
		cScheme, cIdent, cName string
		dScheme, dIdent, dName *string
		reference              *string
	)
	if err := row.Scan(
		&p.DomesticPaymentID, &p.ConsentID, &status, &p.CreationDateTime, &p.StatusUpdateDateTime,
		&idempKey,
		&instrID, &e2eID, &amount, &currency,
		&cScheme, &cIdent, &cName, &dScheme, &dIdent, &dName, &reference,
	); err != nil {
		return nil, err
	}

	p.Status = Status(status)
	p.IdempotencyKey = deref(idempKey)
	p.InstructionIdentification = deref(instrID)
	p.EndToEndIdentification = deref(e2eID)
	p.Reference = deref(reference)

	amt, err := obie.NewAmount(amount, currency)
	if err != nil {
		return nil, err
	}
	p.InstructedAmount = amt

	p.CreditorAccount = Account{SchemeName: cScheme, Identification: cIdent, Name: cName}
	p.DebtorAccount = accountFromCols(dScheme, dIdent, dName)
	return &p, nil
}

func accountCols(a *Account) (scheme, ident, name *string) {
	if a == nil {
		return nil, nil, nil
	}
	return &a.SchemeName, &a.Identification, ptrOrNil(a.Name)
}

func accountFromCols(scheme, ident, name *string) *Account {
	if scheme == nil || ident == nil {
		return nil
	}
	return &Account{SchemeName: *scheme, Identification: *ident, Name: deref(name)}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
