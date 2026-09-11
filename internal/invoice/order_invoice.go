package invoice

import (
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"
)

type Order struct {
	ID          string      `json:"id"`
	Currency    string      `json:"currency"`
	Checkout    Checkout    `json:"checkout"`
	Fulfillment Fulfillment `json:"fulfillment"`
	Receipt     Receipt     `json:"receipt"`
	Items       []LineItem  `json:"items"`
}

type Checkout struct {
	Status string `json:"status"`
}

type Fulfillment struct {
	Status string `json:"status"`
}

type Receipt struct {
	Number        string `json:"number"`
	CustomerEmail string `json:"customer_email"`
}

type LineItem struct {
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	UnitCents int64  `json:"unit_cents"`
}

type Decision struct {
	GeneratePDF    bool   `json:"generate_pdf"`
	CustomerUpdate string `json:"customer_update"`
	Reason         string `json:"reason,omitempty"`
}

func Decide(order Order) Decision {
	if order.Checkout.Status != "paid" {
		return Decision{Reason: "checkout is not paid"}
	}
	if order.Fulfillment.Status != "shipped" && order.Fulfillment.Status != "delivered" {
		return Decision{Reason: "order is not fulfilled"}
	}
	return Decision{GeneratePDF: true, CustomerUpdate: "invoice_ready"}
}

func Validate(order Order) error {
	if strings.TrimSpace(order.ID) == "" || strings.TrimSpace(order.Currency) == "" {
		return errors.New("order id and currency are required")
	}
	if strings.TrimSpace(order.Receipt.Number) == "" || strings.TrimSpace(order.Receipt.CustomerEmail) == "" {
		return errors.New("receipt number and customer email are required")
	}
	if len(order.Items) == 0 {
		return errors.New("at least one line item is required")
	}
	for _, item := range order.Items {
		if strings.TrimSpace(item.Name) == "" || item.Quantity <= 0 || item.UnitCents < 0 {
			return errors.New("line items require a name, positive quantity, and non-negative unit price")
		}
	}
	return nil
}

type invoiceView struct {
	Order      Order
	IssuedDate string
	TotalCents int64
}

const invoiceTemplate = `<!doctype html><html><head><meta charset="utf-8"><style>body{font-family:sans-serif;margin:40px}table{width:100%;border-collapse:collapse}th,td{padding:8px;border-bottom:1px solid #ddd;text-align:left}.amount{text-align:right}</style></head><body><h1>Invoice {{.Order.Receipt.Number}}</h1><p>Order {{.Order.ID}} · Issued {{.IssuedDate}}</p><table><thead><tr><th>Item</th><th>Qty</th><th class="amount">Unit</th></tr></thead><tbody>{{range .Order.Items}}<tr><td>{{.Name}}</td><td>{{.Quantity}}</td><td class="amount">{{.UnitCents}} cents</td></tr>{{end}}</tbody></table><p class="amount"><strong>Total: {{.TotalCents}} {{.Order.Currency}} cents</strong></p></body></html>`

func RenderHTML(order Order, now time.Time) (string, error) {
	var total int64
	for _, item := range order.Items {
		total += int64(item.Quantity) * item.UnitCents
	}
	tmpl, err := template.New("invoice").Parse(invoiceTemplate)
	if err != nil {
		return "", fmt.Errorf("parse invoice template: %w", err)
	}
	var output strings.Builder
	err = tmpl.Execute(&output, invoiceView{Order: order, IssuedDate: now.UTC().Format("2006-01-02"), TotalCents: total})
	if err != nil {
		return "", fmt.Errorf("render invoice: %w", err)
	}
	return output.String(), nil
}
