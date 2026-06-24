package payments

import (
	"testing"

	"github.com/sreeni/openbank-bian/pkg/obie"
)

func TestAmountsEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b obie.Amount
		want bool
	}{
		{"identical", obie.MustAmount("165.88", "GBP"), obie.MustAmount("165.88", "GBP"), true},
		{"trailing zero", obie.MustAmount("165.88", "GBP"), obie.MustAmount("165.880", "GBP"), true},
		{"value differs", obie.MustAmount("165.88", "GBP"), obie.MustAmount("165.89", "GBP"), false},
		{"currency differs", obie.MustAmount("165.88", "GBP"), obie.MustAmount("165.88", "EUR"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := amountsEqual(tc.a, tc.b); got != tc.want {
				t.Fatalf("amountsEqual = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewPaymentDefaultsToInProcess(t *testing.T) {
	// A freshly accepted payment carries the AcceptedSettlementInProcess status.
	// (The service sets this; this guards the constant the service relies on.)
	if StatusAcceptedSettlementInProcess != "AcceptedSettlementInProcess" {
		t.Fatalf("unexpected status constant %q", StatusAcceptedSettlementInProcess)
	}
}
