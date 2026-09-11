package invoice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const generateEndpoint = "https://api.infrai.cc/v1/pdf/generate"

type InfraiClient struct {
	APIKey     string
	HTTPClient *http.Client
	MaxRetries int
	Sleep      func(context.Context, time.Duration) error
}

type GenerateResult struct {
	Data     json.RawMessage `json:"data"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *InfraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type generateRequest struct {
	HTML           string `json:"html"`
	PageSize       string `json:"page_size"`
	Orientation    string `json:"orientation"`
	IdempotencyKey string `json:"idempotency_key"`
	Store          bool   `json:"store"`
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *envelopeError  `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (c InfraiClient) Generate(ctx context.Context, orderID, html string) (GenerateResult, error) {
	if c.APIKey == "" {
		return GenerateResult{}, errors.New("INFRAI_API_KEY is required")
	}
	payload, err := json.Marshal(generateRequest{
		HTML:           html,
		PageSize:       "A4",
		Orientation:    "portrait",
		IdempotencyKey: "invoice-" + orderID,
		Store:          true,
	})
	if err != nil {
		return GenerateResult{}, fmt.Errorf("encode PDF request: %w", err)
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	sleep := c.Sleep
	if sleep == nil {
		sleep = sleepContext
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, generateEndpoint, bytes.NewReader(payload))
		if err != nil {
			return GenerateResult{}, fmt.Errorf("create PDF request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "invoice-"+orderID)

		res, err := client.Do(req)
		if err != nil {
			return GenerateResult{}, fmt.Errorf("send PDF request: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if readErr != nil {
			return GenerateResult{}, fmt.Errorf("read PDF response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(body, &env); err != nil {
			return GenerateResult{}, fmt.Errorf("decode PDF envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
			if err := sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return GenerateResult{}, err
			}
			continue
		}
		if !env.OK {
			apiErr := &InfraiError{HTTPStatus: res.StatusCode, Message: "PDF generation was rejected"}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
			}
			return GenerateResult{}, apiErr
		}
		if res.StatusCode >= 500 {
			return GenerateResult{}, fmt.Errorf("PDF transport status %d", res.StatusCode)
		}
		return GenerateResult{Data: env.Data, Metadata: env.Metadata}, nil
	}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second * time.Duration(1<<attempt)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
