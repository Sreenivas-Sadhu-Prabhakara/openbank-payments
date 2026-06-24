package payments

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sreeni/openbank-bian/pkg/obie"
)

// newTestHandler wires a handler over an in-memory repo and the given fake
// consent client, with the deterministic clock/ids from newTestService.
func newTestHandler(fc *fakeConsent) http.Handler {
	return NewHandler(newTestService(fc), "http://payments.test").Routes()
}

// do issues a request to the handler and returns the recorder. When idemKey is
// non-empty it is set as the x-idempotency-key header.
func do(t *testing.T, h http.Handler, method, path, body, idemKey string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if idemKey != "" {
		r.Header.Set("x-idempotency-key", idemKey)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const paymentBody = `{
	"Data": {
		"ConsentId": "cons-1",
		"Initiation": {
			"InstructionIdentification": "ID412",
			"EndToEndIdentification": "E2E412",
			"InstructedAmount": {"Amount": "165.88", "Currency": "GBP"},
			"CreditorAccount": {"SchemeName": "UK.OBIE.SortCodeAccountNumber", "Identification": "08080021325698", "Name": "ACME Inc"},
			"RemittanceInformation": {"Reference": "FRESCO-101"}
		}
	},
	"Risk": {"PaymentContextCode": "EcommerceGoods"}
}`

func TestDomesticPaymentFlow(t *testing.T) {
	fc := &fakeConsent{view: authorisedView("165.88", "GBP")}
	h := newTestHandler(fc)

	// Create.
	w := do(t, h, http.MethodPost, "/domestic-payments", paymentBody, "idem-1")
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", w.Code, w.Body)
	}
	var created struct {
		Data  domesticPaymentRespData `json:"Data"`
		Risk  json.RawMessage         `json:"Risk"`
		Links obie.Links              `json:"Links"`
	}
	mustDecode(t, w, &created)
	if created.Data.Status != "AcceptedSettlementInProcess" {
		t.Fatalf("status = %s", created.Data.Status)
	}
	if string(created.Risk) != "{}" {
		t.Fatalf("Risk = %s, want {}", created.Risk)
	}
	if created.Data.Initiation.InstructedAmount.String() != "165.88" {
		t.Fatalf("amount = %s", created.Data.Initiation.InstructedAmount)
	}
	id := created.Data.DomesticPaymentID
	if id == "" {
		t.Fatal("missing DomesticPaymentId")
	}
	if created.Links.Self != "http://payments.test/domestic-payments/"+id {
		t.Fatalf("Self = %s", created.Links.Self)
	}
	if fc.consumeCalls != 1 {
		t.Fatalf("Consume called %d times, want 1", fc.consumeCalls)
	}

	// Get.
	w = do(t, h, http.MethodGet, "/domestic-payments/"+id, "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d, body=%s", w.Code, w.Body)
	}
	var got struct {
		Data domesticPaymentRespData `json:"Data"`
	}
	mustDecode(t, w, &got)
	if got.Data.DomesticPaymentID != id || got.Data.Initiation.RemittanceInformation == nil ||
		got.Data.Initiation.RemittanceInformation.Reference != "FRESCO-101" {
		t.Fatalf("unexpected get body %+v", got.Data)
	}

	// Payment details.
	w = do(t, h, http.MethodGet, "/domestic-payments/"+id+"/payment-details", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("details status = %d, body=%s", w.Code, w.Body)
	}
	var details struct {
		Data paymentDetailsRespData `json:"Data"`
	}
	mustDecode(t, w, &details)
	if len(details.Data.PaymentStatus) != 1 ||
		details.Data.PaymentStatus[0].PaymentStatusCode != "AcceptedSettlementInProcess" ||
		details.Data.PaymentStatus[0].Status != "AcceptedSettlementInProcess" {
		t.Fatalf("unexpected payment-details %+v", details.Data.PaymentStatus)
	}
}

func TestCreateMissingIdempotencyKey(t *testing.T) {
	fc := &fakeConsent{view: authorisedView("165.88", "GBP")}
	h := newTestHandler(fc)

	w := do(t, h, http.MethodPost, "/domestic-payments", paymentBody, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body)
	}
	var errBody obie.ErrorResponse
	mustDecode(t, w, &errBody)
	if len(errBody.Errors) == 0 || errBody.Errors[0].ErrorCode != obie.ErrHeaderMissing {
		t.Fatalf("unexpected error body %+v", errBody)
	}
}

func TestCreateIdempotentReplayOverHTTP(t *testing.T) {
	fc := &fakeConsent{view: authorisedView("165.88", "GBP")}
	h := newTestHandler(fc)

	w1 := do(t, h, http.MethodPost, "/domestic-payments", paymentBody, "idem-1")
	w2 := do(t, h, http.MethodPost, "/domestic-payments", paymentBody, "idem-1")
	if w1.Code != http.StatusCreated || w2.Code != http.StatusCreated {
		t.Fatalf("statuses = %d, %d", w1.Code, w2.Code)
	}
	var p1, p2 struct {
		Data domesticPaymentRespData `json:"Data"`
	}
	mustDecode(t, w1, &p1)
	mustDecode(t, w2, &p2)
	if p1.Data.DomesticPaymentID != p2.Data.DomesticPaymentID {
		t.Fatalf("replay produced different ids: %s != %s", p1.Data.DomesticPaymentID, p2.Data.DomesticPaymentID)
	}
	if fc.consumeCalls != 1 {
		t.Fatalf("Consume called %d times, want 1", fc.consumeCalls)
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	h := newTestHandler(&fakeConsent{view: authorisedView("165.88", "GBP")})
	w := do(t, h, http.MethodPost, "/domestic-payments",
		`{"Data":{"ConsentId":"cons-1","Initiation":{}},"Risk":{},"Bogus":true}`, "idem-1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body)
	}
}

func TestGetUnknownPaymentOverHTTP(t *testing.T) {
	h := newTestHandler(&fakeConsent{})
	w := do(t, h, http.MethodGet, "/domestic-payments/nope", "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
	var errBody obie.ErrorResponse
	mustDecode(t, w, &errBody)
	if errBody.Code != "Not Found" || len(errBody.Errors) == 0 {
		t.Fatalf("unexpected error body %+v", errBody)
	}
}

func TestHealth(t *testing.T) {
	h := newTestHandler(&fakeConsent{})
	w := do(t, h, http.MethodGet, "/health", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("health status = %d", w.Code)
	}
}

func mustDecode(t *testing.T, w *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
}
