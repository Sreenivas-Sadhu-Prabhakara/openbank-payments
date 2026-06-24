package payments

import (
	"encoding/json"
	"time"

	"github.com/sreeni/openbank-bian/pkg/obie"
)

// envelope is the OBIE payment response shape. It mirrors obie.Response but adds
// the Risk object that payment resources carry.
type envelope struct {
	Data  any             `json:"Data"`
	Risk  json.RawMessage `json:"Risk"`
	Links obie.Links      `json:"Links"`
	Meta  obie.Meta       `json:"Meta"`
}

// emptyRisk is returned where we accept but do not persist the Risk block.
var emptyRisk = json.RawMessage(`{}`)

func newEnvelope(self string, data any) envelope {
	return envelope{Data: data, Risk: emptyRisk, Links: obie.Links{Self: self}, Meta: obie.Meta{}}
}

// accountDTO is the OBIE account identifier block on the wire.
type accountDTO struct {
	SchemeName     string `json:"SchemeName"`
	Identification string `json:"Identification"`
	Name           string `json:"Name,omitempty"`
}

func (d accountDTO) toDomain() Account {
	return Account{SchemeName: d.SchemeName, Identification: d.Identification, Name: d.Name}
}

func accountToDTO(a *Account) *accountDTO {
	if a == nil {
		return nil
	}
	return &accountDTO{SchemeName: a.SchemeName, Identification: a.Identification, Name: a.Name}
}

// remittanceDTO is the OBIE RemittanceInformation block carrying the payment
// reference.
type remittanceDTO struct {
	Reference    string `json:"Reference,omitempty"`
	Unstructured string `json:"Unstructured,omitempty"`
}

// initiationDTO mirrors the domestic-payment-consent Initiation shape exactly,
// so a payment is submitted with the same instruction the consent authorised.
type initiationDTO struct {
	InstructionIdentification string         `json:"InstructionIdentification"`
	EndToEndIdentification    string         `json:"EndToEndIdentification"`
	InstructedAmount          obie.Amount    `json:"InstructedAmount"`
	CreditorAccount           accountDTO     `json:"CreditorAccount"`
	DebtorAccount             *accountDTO    `json:"DebtorAccount,omitempty"`
	RemittanceInformation     *remittanceDTO `json:"RemittanceInformation,omitempty"`
}

// ---- POST /domestic-payments request ----
//
// Every field is declared (incl. Risk and RemittanceInformation) so
// DecodeJSON's DisallowUnknownFields does not reject a valid OBIE body.
type domesticPaymentReq struct {
	Data struct {
		ConsentID  string        `json:"ConsentId"`
		Initiation initiationDTO `json:"Initiation"`
	} `json:"Data"`
	Risk json.RawMessage `json:"Risk"`
}

// ---- response ----

type domesticPaymentRespData struct {
	DomesticPaymentID    string        `json:"DomesticPaymentId"`
	ConsentID            string        `json:"ConsentId"`
	Status               string        `json:"Status"`
	CreationDateTime     string        `json:"CreationDateTime"`
	StatusUpdateDateTime string        `json:"StatusUpdateDateTime"`
	Initiation           initiationDTO `json:"Initiation"`
}

func domesticPaymentData(p *DomesticPayment) domesticPaymentRespData {
	init := initiationDTO{
		InstructionIdentification: p.InstructionIdentification,
		EndToEndIdentification:    p.EndToEndIdentification,
		InstructedAmount:          p.InstructedAmount,
		CreditorAccount:           *accountToDTO(&p.CreditorAccount),
		DebtorAccount:             accountToDTO(p.DebtorAccount),
	}
	if p.Reference != "" {
		init.RemittanceInformation = &remittanceDTO{Reference: p.Reference}
	}
	return domesticPaymentRespData{
		DomesticPaymentID:    p.DomesticPaymentID,
		ConsentID:            p.ConsentID,
		Status:               string(p.Status),
		CreationDateTime:     rfc3339(p.CreationDateTime),
		StatusUpdateDateTime: rfc3339(p.StatusUpdateDateTime),
		Initiation:           init,
	}
}

// ---- GET /domestic-payments/{id}/payment-details ----

// paymentStatusDetailDTO is one entry in the OBIE PaymentStatus array returned
// by the payment-details endpoint.
type paymentStatusDetailDTO struct {
	PaymentStatusCode               string `json:"PaymentStatusCode"`
	PaymentStatusCodeChangeDateTime string `json:"PaymentStatusCodeChangeDateTime"`
	Status                          string `json:"Status"`
}

type paymentDetailsRespData struct {
	PaymentStatus []paymentStatusDetailDTO `json:"PaymentStatus"`
}

func paymentDetailsData(p *DomesticPayment) paymentDetailsRespData {
	return paymentDetailsRespData{
		PaymentStatus: []paymentStatusDetailDTO{{
			PaymentStatusCode:               string(p.Status),
			PaymentStatusCodeChangeDateTime: rfc3339(p.StatusUpdateDateTime),
			Status:                          string(p.Status),
		}},
	}
}

// ---- shared helpers ----

func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339) }
