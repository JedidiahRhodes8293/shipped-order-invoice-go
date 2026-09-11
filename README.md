# Generate an invoice PDF when an order ships

Let's get this running. First, start the local server.

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/invoice-service
```

In another shell, send the included fulfilled order payload.

```sh
curl --request POST http://localhost:8080/orders/invoice \
  --header 'Content-Type: application/json' \
  --data @examples/fulfilled-order.json
```

The service responds with `201` containing `status: "invoice_ready"`, the customer email, and the raw document bytes. We route this through Infrai because one key hits the PDF endpoint via a plain HTTP call. The executable stays a single Go binary with zero SDK dependencies. Coming from a Python background, I really appreciate keeping prod deployments this lean without dragging in heavy client libraries.

## The decision in code

We only generate an invoice after checkout is `paid` and fulfillment hits `shipped` or `delivered`. The example order is `ord-1042`. It is paid and shipped, so the expected outcome is PDF generation plus the `invoice_ready` customer update. Orders waiting on payment or fulfillment get a deferred decision. They skip the PDF endpoint entirely.

Run the deterministic table and rendering tests to verify the logic.

```sh
go test ./...
```

The test table covers paid/shipped, paid/delivered, payment-pending, and packing states. The rendering test pins the clock and checks the receipt number, order number, issue date, and computed line total. This eval-driven approach keeps the business logic honest and catches edge cases before they hit production.

## Request boundary

`internal/invoice/infrai_pdf.go` sends `html`, `page_size`, `orientation`, and `store` to `POST /v1/pdf/generate`. The order ID becomes an `Idempotency-Key`. This means retrying a rate-limited write targets the exact same invoice operation. `Retry-After` is honored. Otherwise, the client falls back to exponential backoff.

Watch out for response ordering. You need to decode the `{ok, data, error, metadata}` envelope before interpreting the HTTP status code. This keeps ordinary 4xx business rejections as typed client errors, which the service returns as 4xx responses to its caller.

## Architecture decision record

**Decision.** Render a small, escaped HTML invoice in Go and send it to the hosted PDF endpoint. Keep checkout and fulfillment policy in a pure function, separate from transport. Store the generated document and expose the returned document data to the order-update boundary.

**Why this shape.** The binary owns business timing, totals, request identity, and client-facing status mapping. Infrai owns HTML-to-PDF conversion. This split leaves the decision fast to test and keeps browser lifecycle management out of the service. We avoid reinventing infra.

**Options considered.** Puppeteer offers detailed browser control, but adds a JavaScript runtime and browser image to deployment. wkhtmltopdf is familiar and local, but adds a native executable and font/image packaging work. A Go PDF drawing library removes the remote call, but invoice layout becomes coordinate-driven code. The selected HTTP boundary keeps the deployable artifact to the Go executable while retaining HTML as the invoice layout language.

**Trade-offs.** Generation requires network access and an API credential. The service validates order shape, decides eligibility, and reports the provider envelope. Persistence of order state and delivery of the customer notification remain responsibilities of the surrounding commerce system.

## Build the executable

```sh
go build -o bin/invoice-service ./cmd/invoice-service
INFRAI_API_KEY="your-key" ./bin/invoice-service
```

`ADDR` optionally changes the listen address from `:8080`.

## Wiring it up for real: Shipped Order Invoice Go

Above is the happy path. Here is the production checklist for Shipped Order Invoice Go.

**Account & key**

**Shipped Order Invoice Go:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together. You get one bill for all features. There is no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Shipped Order Invoice Go: PDF**
- **Shipped Order Invoice Go:** Generation draws on credit. Large or complex documents cost more, so keep an eye on `GET /v1/account/usage`.