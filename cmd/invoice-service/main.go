package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"example.com/order-invoice-service/internal/invoice"
)

type server struct {
	client invoice.InfraiClient
	now    func() time.Time
}

type invoiceResponse struct {
	OrderID       string          `json:"order_id"`
	Status        string          `json:"status"`
	CustomerEmail string          `json:"customer_email,omitempty"`
	Document      json.RawMessage `json:"document,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	Reason        string          `json:"reason,omitempty"`
}

func (s server) createInvoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}
	var order invoice.Order
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&order); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid order JSON"})
		return
	}
	if err := invoice.Validate(order); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	decision := invoice.Decide(order)
	if !decision.GeneratePDF {
		writeJSON(w, http.StatusConflict, invoiceResponse{OrderID: order.ID, Status: "invoice_deferred", Reason: decision.Reason})
		return
	}
	html, err := invoice.RenderHTML(order, s.now())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invoice rendering failed"})
		return
	}
	result, err := s.client.Generate(r.Context(), order.ID, html)
	if err != nil {
		var apiErr *invoice.InfraiError
		if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			writeJSON(w, apiErr.HTTPStatus, map[string]string{"error": apiErr.Error()})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invoice provider request failed"})
		return
	}
	writeJSON(w, http.StatusCreated, invoiceResponse{OrderID: order.ID, Status: decision.CustomerUpdate, CustomerEmail: order.Receipt.CustomerEmail, Document: result.Data, Metadata: result.Metadata})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	s := server{client: invoice.InfraiClient{APIKey: apiKey, MaxRetries: 3}, now: time.Now}
	mux := http.NewServeMux()
	mux.HandleFunc("/orders/invoice", s.createInvoice)
	log.Printf("invoice service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
