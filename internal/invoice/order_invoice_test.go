package invoice

import (
	"strings"
	"testing"
	"time"
)

func TestDecideInvoice(t *testing.T) {
	tests := []struct {
		name        string
		checkout    string
		fulfillment string
		wantPDF     bool
		wantUpdate  string
	}{
		{name: "paid and shipped", checkout: "paid", fulfillment: "shipped", wantPDF: true, wantUpdate: "invoice_ready"},
		{name: "paid and delivered", checkout: "paid", fulfillment: "delivered", wantPDF: true, wantUpdate: "invoice_ready"},
		{name: "payment pending", checkout: "pending", fulfillment: "shipped"},
		{name: "packing", checkout: "paid", fulfillment: "packing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(Order{Checkout: Checkout{Status: tt.checkout}, Fulfillment: Fulfillment{Status: tt.fulfillment}})
			if got.GeneratePDF != tt.wantPDF || got.CustomerUpdate != tt.wantUpdate {
				t.Fatalf("Decide() = %+v, want PDF=%v update=%q", got, tt.wantPDF, tt.wantUpdate)
			}
		})
	}
}

func TestRenderHTMLTotalsReceipt(t *testing.T) {
	order := Order{ID: "ord-1042", Currency: "USD", Receipt: Receipt{Number: "R-1042"}, Items: []LineItem{{Name: "Cable", Quantity: 2, UnitCents: 1250}}}
	got, err := RenderHTML(order, time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Invoice R-1042", "Order ord-1042", "2500 USD cents", "2026-08-26"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered invoice missing %q", want)
		}
	}
}
