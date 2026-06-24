package payments

import (
	"net/http"

	"github.com/sreeni/openbank-bian/pkg/httpx"
)

// idempotencyHeader is the OBIE-mandatory header on payment creation. A retried
// POST with the same value must not create a duplicate payment.
const idempotencyHeader = "x-idempotency-key"

// Handler exposes the payments service over HTTP using OBIE PISP request/response
// shapes. baseURL is used to build absolute Self links.
type Handler struct {
	svc     *Service
	baseURL string
}

// NewHandler constructs the HTTP handler.
func NewHandler(svc *Service, baseURL string) *Handler {
	return &Handler{svc: svc, baseURL: baseURL}
}

// Routes registers every payment route on a ServeMux and returns it.
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	// Domestic payments (PISP).
	mux.HandleFunc("POST /domestic-payments", h.createDomesticPayment)
	mux.HandleFunc("GET /domestic-payments/{domesticPaymentId}", h.getDomesticPayment)
	mux.HandleFunc("GET /domestic-payments/{domesticPaymentId}/payment-details", h.getPaymentDetails)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

func (h *Handler) self(r *http.Request) string { return h.baseURL + r.URL.Path }

func (h *Handler) createDomesticPayment(w http.ResponseWriter, r *http.Request) {
	var req domesticPaymentReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.RespondError(w, err)
		return
	}
	init := req.Data.Initiation
	in := CreateInput{
		IdempotencyKey:            r.Header.Get(idempotencyHeader),
		ConsentID:                 req.Data.ConsentID,
		InstructionIdentification: init.InstructionIdentification,
		EndToEndIdentification:    init.EndToEndIdentification,
		InstructedAmount:          init.InstructedAmount,
		CreditorAccount:           init.CreditorAccount.toDomain(),
	}
	if init.DebtorAccount != nil {
		da := init.DebtorAccount.toDomain()
		in.DebtorAccount = &da
	}
	if init.RemittanceInformation != nil {
		in.Reference = init.RemittanceInformation.Reference
	}

	p, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.RespondError(w, err)
		return
	}
	self := h.baseURL + "/domestic-payments/" + p.DomesticPaymentID
	httpx.WriteJSON(w, http.StatusCreated, newEnvelope(self, domesticPaymentData(p)))
}

func (h *Handler) getDomesticPayment(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Get(r.Context(), r.PathValue("domesticPaymentId"))
	if err != nil {
		httpx.RespondError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newEnvelope(h.self(r), domesticPaymentData(p)))
}

func (h *Handler) getPaymentDetails(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Get(r.Context(), r.PathValue("domesticPaymentId"))
	if err != nil {
		httpx.RespondError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, newEnvelope(h.self(r), paymentDetailsData(p)))
}
