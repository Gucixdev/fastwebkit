// Crypto checkout via a self-hosted BTCPay Server instance - fits "self-hosted" better than a
// custodial processor, and BTCPay's Greenfield REST API is simple enough that a thin client
// here beats depending on one of the handful of small, inconsistently-maintained community Go
// SDKs for it (checked: none looked solid enough to depend on).

package kit

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NewCrypto wires a BTCPay Server provider. serverURL is the BTCPay instance's base URL
// (e.g. https://btcpay.example.com), storeID and apiKey come from that instance, webhookSecret
// is set when creating the webhook in the BTCPay store settings. All from env vars in
// cmd/fastwebapp - never hardcode them.
func NewCrypto(serverURL, storeID, apiKey, webhookSecret string) Provider {
	return cryptoProvider{
		serverURL:     strings.TrimSuffix(serverURL, "/"),
		storeID:       storeID,
		apiKey:        apiKey,
		webhookSecret: webhookSecret,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
	}
}

type cryptoProvider struct {
	serverURL     string
	storeID       string
	apiKey        string
	webhookSecret string
	httpClient    *http.Client
}

func (c cryptoProvider) Name() string { return "crypto" }

func (c cryptoProvider) CreateCheckout(checkout Checkout) (string, error) {
	if err := checkout.Validate(); err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]any{
		"amount":   centsToDecimal(checkout.PriceCents),
		"currency": upper(checkout.Currency),
		"metadata": map[string]string{"itemSlug": checkout.ItemSlug},
		"checkout": map[string]any{
			"redirectURL":           checkout.SuccessURL,
			"redirectAutomatically": true,
		},
	})

	var invoice struct {
		CheckoutLink string `json:"checkoutLink"`
	}
	path := fmt.Sprintf("/api/v1/stores/%s/invoices", c.storeID)
	if err := c.do(http.MethodPost, path, body, &invoice); err != nil {
		return "", fmt.Errorf("btcpay: create invoice: %w", err)
	}
	return invoice.CheckoutLink, nil
}

func (c cryptoProvider) VerifyWebhook(r *http.Request) (bool, string, error) {
	p, err := c.VerifyPayment(r)
	return p.Paid, p.ItemSlug, err
}

func (c cryptoProvider) VerifyPayment(r *http.Request) (Payment, error) {
	if c.webhookSecret == "" {
		return Payment{}, fmt.Errorf("btcpay: webhook secret not configured")
	}
	payload, err := readBody(r)
	if err != nil {
		return Payment{}, fmt.Errorf("btcpay: read body: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(c.webhookSecret))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(r.Header.Get("BTCPay-Sig"))) {
		return Payment{}, fmt.Errorf("btcpay: invalid webhook signature")
	}

	var event struct {
		Type      string `json:"type"`
		InvoiceID string `json:"invoiceId"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return Payment{}, fmt.Errorf("btcpay: decode event: %w", err)
	}
	switch event.Type {
	case "InvoiceInvalid", "InvoiceExpired":
		// Invalid/Expired after Settled (reorg, chargeback-like reversal) withdraws the grant. On an
		// invoice that never settled the ref is simply unknown to the ledger, so this is harmless.
		// Trust the server, not the event: re-fetch and require the invoice really is Invalid/Expired.
		var inv struct {
			Status string `json:"status"`
		}
		path := fmt.Sprintf("/api/v1/stores/%s/invoices/%s", c.storeID, url.PathEscape(event.InvoiceID))
		if err := c.do(http.MethodGet, path, nil, &inv); err != nil {
			return Payment{}, fmt.Errorf("btcpay: fetch invoice: %w", err)
		}
		if inv.Status == "Invalid" || inv.Status == "Expired" {
			return Payment{Revoke: true, Ref: event.InvoiceID}, nil
		}
		return Payment{}, nil
	case "InvoiceSettled":
	default:
		return Payment{}, nil
	}

	// The webhook only carries the invoice id, not its metadata - fetch the invoice to read the
	// item slug back, per BTCPay's own documented pattern for correlating webhooks to orders.
	var invoice struct {
		Status   string      `json:"status"`
		Amount   json.Number `json:"amount"`
		Currency string      `json:"currency"`
		Metadata struct {
			ItemSlug string `json:"itemSlug"`
		} `json:"metadata"`
	}
	path := fmt.Sprintf("/api/v1/stores/%s/invoices/%s", c.storeID, url.PathEscape(event.InvoiceID))
	if err := c.do(http.MethodGet, path, nil, &invoice); err != nil {
		return Payment{}, fmt.Errorf("btcpay: fetch invoice: %w", err)
	}
	// Trust the server, not the (signed but replayable) event: the invoice must really be settled.
	if invoice.Status != "" && invoice.Status != "Settled" {
		return Payment{}, nil
	}
	cents, ok := decimalToCents(invoice.Amount.String())
	return Payment{Paid: true, ItemSlug: invoice.Metadata.ItemSlug, Known: ok, Ref: event.InvoiceID,
		AmountCents: cents, Currency: strings.ToLower(invoice.Currency)}, nil
}

func (c cryptoProvider) do(method, path string, body []byte, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.serverURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, respBody)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
