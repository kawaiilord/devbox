package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"
)

type MailMessage struct {
	Type      string    `json:"type"`
	To        string    `json:"to"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Mailer interface {
	Send(context.Context, MailMessage) error
}

type WebhookMailer struct {
	url    string
	secret string
	client *http.Client
}

func NewWebhookMailer(url, secret string) (*WebhookMailer, error) {
	if url == "" || secret == "" {
		return nil, errors.New("mail webhook URL and secret are required")
	}
	return &WebhookMailer{
		url: url, secret: secret, client: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (m *WebhookMailer) Send(ctx context.Context, message MailMessage) error {
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, m.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+m.secret)
	response, err := m.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("mail webhook rejected message")
	}
	return nil
}

type MemoryMailer struct {
	mu       sync.Mutex
	messages []MailMessage
}

func (m *MemoryMailer) Send(_ context.Context, message MailMessage) error {
	m.mu.Lock()
	m.messages = append(m.messages, message)
	m.mu.Unlock()
	return nil
}

func (m *MemoryMailer) LastMessage() (MailMessage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.messages) == 0 {
		return MailMessage{}, false
	}
	return m.messages[len(m.messages)-1], true
}

type NoopMailer struct{}

func (NoopMailer) Send(context.Context, MailMessage) error { return nil }
