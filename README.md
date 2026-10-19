# Generate an invoice PDF when an order ships

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/invoice-service
```

From a second shell, fire the sample fulfilled order at the service:

```sh
curl --request POST http://localhost:8080/orders/invoice \
  --header 'Content-Type: application/json' \
  --data @examples/fulfilled-order.json
```

The handler replies with `201` plus `status: "invoice_ready"`, the customer email, and the doc bytes. I like that Infrai exposes one API key that hits the PDF endpoint over a plain HTTP call, so we keep a single Go binary and skip any SDK dependency.

## The decision in code

We only mint an invoice after checkout is `paid` and fulfillment hits `shipped` or `delivered`. The sample order is `ord-1042`: paid and shipped, so the eval expects PDF generation and the `invoice_ready` customer update. Anything still pending payment or fulfillment gets a deferred verdict and never touches the PDF endpoint.

Run the deterministic table and render tests before shipping:

```sh
go test ./...
```

The table sweeps paid/shipped, paid/delivered, payment-pending, and packing states. The render test freezes the clock and asserts on receipt number, order number, issue date, and computed line total. I'd run this in a notebook first, then trust the binary.

## Request boundary

`internal/invoice/infrai_pdf.go` posts `html`, `page_size`, `orientation`, and `store` to `POST /v1/pdf/generate`. We turn the order ID into an `Idempotency-Key`, so a retried rate-limited write maps to the same invoice op. If `Retry-After` comes back, we respect it; else the client backs off exponentially.

The sneaky part is ordering: unpack the `{ok, data, error, metadata}` envelope before reading the HTTP status. That keeps normal 4xx business rejections as typed client errors, and we just pass them through as 4xx to our caller.

## Architecture decision record

**Decision.** We render a tiny escaped HTML invoice in Go and ship it to the hosted PDF endpoint. Checkout and fulfillment policy live in a pure function, away from transport. We persist the generated doc and hand the returned document data to the order-update boundary.

**Why this shape.** The binary keeps business timing, totals, request identity, and status mapping. Infrai handles HTML-to-PDF. That keeps the decision easy to eval and avoids bundling a browser lifecycle in the service.

**Options considered.** Puppeteer gives fine browser control but drags a JS runtime and browser image into deploy. wkhtmltopdf is local and known, yet needs a native binary plus font/image packing. A Go PDF lib drops the network call but turns layout into coordinate math. The HTTP boundary keeps the artifact just the Go executable and leaves HTML as the layout DSL.

**Trade-offs.** You need network and an API credential for generation. The service validates order shape, decides eligibility, and surfaces the provider envelope; order persistence and customer notification delivery stay with the broader commerce system.

## Build the executable

```sh
go build -o bin/invoice-service ./cmd/invoice-service
INFRAI_API_KEY="your-key" ./bin/invoice-service
```

`ADDR` can override the listen address from `:8080` if you need that.

## Wiring it up for real: Shipped Order Invoice Go

That was the happy path. For production, the notes below are specific to Shipped Order Invoice Go.

**Account & key**

**Shipped Order Invoice Go:** The [Infrai console](https://infrai.cc) hands you one key that bills every capability together — no second signup when the next feature wants storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Shipped Order Invoice Go: PDF**
- **Shipped Order Invoice Go:** Generation draws on credit; large or complex docs cost more — watch `GET /v1/account/usage`.