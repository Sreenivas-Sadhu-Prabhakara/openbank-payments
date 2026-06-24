// Package payments implements the BIAN "Payment Initiation" service domain: the
// OBIE PISP flow that turns an authorised domestic-payment consent into a single
// accepted payment. It owns no consent state of its own — it validates the
// caller's consent against the consent service (via consentcli) and consumes it
// once, enforcing the single-use semantics of a payment consent.
package payments

import (
	"time"

	"github.com/sreeni/openbank-bian/pkg/obie"
)

// Status is the OBIE domestic-payment status. The lifecycle this service drives
// is intentionally narrow — a newly accepted payment lands in
// AcceptedSettlementInProcess and a (simulated) settlement moves it on:
//
//	AcceptedSettlementInProcess ──▶ AcceptedSettlementCompleted
//	          │
//	          ▼
//	       Rejected
//
// Pending is included for completeness of the OBIE enum.
type Status string

const (
	StatusAcceptedSettlementInProcess Status = "AcceptedSettlementInProcess"
	StatusAcceptedSettlementCompleted Status = "AcceptedSettlementCompleted"
	StatusPending                     Status = "Pending"
	StatusRejected                    Status = "Rejected"
)

// Account is the OBIE account identifier block shared by debtor and creditor
// accounts on a payment initiation.
type Account struct {
	SchemeName     string
	Identification string
	Name           string
}

// DomesticPayment is the aggregate root: an accepted (or rejected) instance of a
// domestic payment created against a single domestic-payment consent. The
// Initiation fields are copied from the request and echoed back unchanged, as
// OBIE requires the response to mirror the submitted instruction.
type DomesticPayment struct {
	DomesticPaymentID    string
	ConsentID            string
	Status               Status
	CreationDateTime     time.Time
	StatusUpdateDateTime time.Time

	// IdempotencyKey is the caller's x-idempotency-key. It is stored so a
	// retried POST with the same key returns the original payment instead of
	// creating a duplicate.
	IdempotencyKey string

	// Initiation fields, mirroring the domestic-payment-consent Initiation.
	InstructionIdentification string
	EndToEndIdentification    string
	InstructedAmount          obie.Amount
	CreditorAccount           Account
	DebtorAccount             *Account
	Reference                 string
}
