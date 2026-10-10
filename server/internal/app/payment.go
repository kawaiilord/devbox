package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type PaymentProvider interface {
	Name() string
	CreateIntent(context.Context, Order) (PaymentIntent, error)
	VerifyCallback(orderNo, tradeNo string, amountMinor, timestamp int64, signature string) bool
}

type HMACPaymentProvider struct {
	checkout *url.URL
	secret   []byte
	name     string
	now      func() time.Time
}

func NewHMACPaymentProvider(name, checkoutURL, secret string) (*HMACPaymentProvider, error) {
	parsed, err := url.Parse(strings.TrimSpace(checkoutURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil {
		return nil, errors.New("invalid payment checkout URL")
	}
	if len(secret) < 32 {
		return nil, errors.New("payment callback secret must be at least 32 characters")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "hmac"
	}
	return &HMACPaymentProvider{checkout: parsed, secret: []byte(secret), name: name, now: time.Now}, nil
}

func (p *HMACPaymentProvider) Name() string { return p.name }

func (p *HMACPaymentProvider) CreateIntent(_ context.Context, order Order) (PaymentIntent, error) {
	value := *p.checkout
	query := value.Query()
	query.Set("order_no", order.OrderNo)
	query.Set("amount_minor", strconv.FormatInt(order.AmountMinor, 10))
	query.Set("currency", order.Currency)
	value.RawQuery = query.Encode()
	return PaymentIntent{CheckoutURL: value.String(), ExpiresAt: order.QRExpiresAt}, nil
}

func (p *HMACPaymentProvider) VerifyCallback(orderNo, tradeNo string, amountMinor, timestamp int64, signature string) bool {
	if orderNo == "" || tradeNo == "" || amountMinor < 0 || timestamp <= 0 || absDuration(p.now().Sub(time.Unix(timestamp, 0))) > 5*time.Minute {
		return false
	}
	expected := p.sign(orderNo, tradeNo, amountMinor, timestamp)
	provided, err := hex.DecodeString(strings.TrimSpace(signature))
	return err == nil && hmac.Equal([]byte(expected), []byte(hex.EncodeToString(provided)))
}

func (p *HMACPaymentProvider) sign(orderNo, tradeNo string, amountMinor, timestamp int64) string {
	mac := hmac.New(sha256.New, p.secret)
	fmt.Fprintf(mac, "%s\n%s\n%d\n%d", orderNo, tradeNo, amountMinor, timestamp)
	return hex.EncodeToString(mac.Sum(nil))
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
