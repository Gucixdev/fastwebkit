package main

import (
	"bytes"
	"io"
	"log"
	"net/http"

	"fastwebapp/gems/payments"
)

// priceLookup returns what an item costs; ok is false for a slug we do not sell.
type priceLookup func(slug string) (priceCents int64, currency string, ok bool)

// webhookHandler is the payment webhook: verify the provider signature, then either grant
// (paid, right amount/currency, known item, exactly once) or revoke (refund/chargeback).
func webhookHandler(provider payments.Provider, ledger *payments.Ledger, price priceLookup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		pay, err := provider.VerifyPayment(r)
		if err != nil {
			log.Printf("webhook %s rejected: %v", provider.Name(), err)
			http.Error(w, "rejected", http.StatusBadRequest)
			return
		}
		if pay.Revoke {
			first, err := ledger.Revoke(provider.Name(), pay.Ref)
			if err != nil {
				log.Printf("ledger revoke: %v", err)
				http.Error(w, "try again", http.StatusInternalServerError)
				return
			}
			if first {
				log.Println("revoked:", pay.Ref)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if pay.Paid {
			slug := pay.ItemSlug
			priceCents, currency, ok := price(slug)
			// Signature proves the provider sent it; this proves it was paid in full, in our currency,
			// for an item we sell. 200 so the provider stops retrying - a retry cannot fix an underpayment.
			if !ok || !pay.Matches(priceCents, currency) {
				log.Printf("payment REFUSED slug=%q known=%v amount=%d %s (want %d %s)", slug, pay.Known, pay.AmountCents, pay.Currency, priceCents, currency)
				w.WriteHeader(http.StatusOK)
				return
			}
			first, err := ledger.RecordRef(provider.Name(), body, slug, pay.Ref)
			if err != nil {
				// 500 makes the provider retry; nothing is lost, the ledger is the source of truth.
				log.Printf("ledger: %v", err)
				http.Error(w, "try again", http.StatusInternalServerError)
				return
			}
			if first {
				log.Println("paid:", slug)
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}
