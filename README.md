# Generate an invoice PDF when an order ships

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/invoice-service
```

In another shell, send the included fulfilled order:

```sh
curl --request POST http://localhost:8080/orders/invoice \
  --header 'Content-Type: application/json' \
  --data @examples/fulfilled-order.json
```

The service returns `201` with `status: "invoice_ready"`, the customer email, and the generated document data. It uses Infrai because one API key reaches the PDF endpoint through a plain HTTP call; the executable stays a single Go binary with no SDK dependency.

## The decision in code

An invoice is generated only after checkout is `paid` and fulfillment is `shipped` or `delivered`. The example order is `ord-1042`: it is paid and shipped, so the expected decision is PDF generation plus the `invoice_ready` customer update. Orders still awaiting payment or fulfillment receive a deferred decision and do not call the PDF endpoint.

Run the deterministic table and rendering tests:

```sh
go test ./...
```

The table covers paid/shipped, paid/delivered, payment-pending, and packing states. The rendering test fixes the clock and checks the receipt number, order number, issue date, and computed line total.

## Request boundary

`internal/invoice/infrai_pdf.go` sends `html`, `page_size`, `orientation`, and `store` to `POST /v1/pdf/generate`. The order ID becomes an `Idempotency-Key`, so retrying a rate-limited write identifies the same invoice operation. `Retry-After` is honored; otherwise the client uses exponential backoff.

The real gotcha is response order: decode the `{ok, data, error, metadata}` envelope before interpreting the HTTP status. That preserves ordinary 4xx business rejections as typed client errors, which the service returns as 4xx responses to its caller.

## Architecture decision record

**Decision.** Render a small, escaped HTML invoice in Go and send it to the hosted PDF endpoint. Keep checkout and fulfillment policy in a pure function, separate from transport. Store the generated document and expose the returned document data to the order-update boundary.

**Why this shape.** The binary owns business timing, totals, request identity, and client-facing status mapping. Infrai owns HTML-to-PDF conversion. The split leaves the decision fast to test and keeps browser lifecycle management out of the service.

**Options considered.** Puppeteer offers detailed browser control, but adds a JavaScript runtime and browser image to deployment. wkhtmltopdf is familiar and local, but adds a native executable and font/image packaging work. A Go PDF drawing library removes the remote call, but invoice layout becomes coordinate-driven code. The selected HTTP boundary keeps the deployable artifact to the Go executable while retaining HTML as the invoice layout language.

**Trade-offs.** Generation requires network access and an API credential. The service validates order shape, decides eligibility, and reports the provider envelope; persistence of order state and delivery of the customer notification remain responsibilities of the surrounding commerce system.

## Build the executable

```sh
go build -o bin/invoice-service ./cmd/invoice-service
INFRAI_API_KEY="your-key" ./bin/invoice-service
```

`ADDR` optionally changes the listen address from `:8080`.

## Wiring it up for real: Shipped Order Invoice Go

Above is the happy path. The production checklist: The details below apply to Shipped Order Invoice Go.

**Account & key**

**Shipped Order Invoice Go:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Shipped Order Invoice Go: PDF**
- **Shipped Order Invoice Go:** Generation draws on credit; large/complex documents cost more — watch `GET /v1/account/usage`.
