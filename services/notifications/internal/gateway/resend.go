// Package gateway holds the email provider adapter (issue #29). Resend's
// REST API is a single POST; the SDK would be a dependency for one call.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/notifications/internal/service"
)

// ResendSender implements the EmailSender port against api.resend.com.
// Until a sending domain is verified in Resend, from must stay
// onboarding@resend.dev and delivery works only to the account owner.
type ResendSender struct {
	apiKey string
	from   string
	client *http.Client
	base   string
}

var _ service.EmailSender = (*ResendSender)(nil)

// NewResendSender constructs the adapter.
func NewResendSender(apiKey, from string) *ResendSender {
	return &ResendSender{
		apiKey: apiKey,
		from:   from,
		client: &http.Client{Timeout: 10 * time.Second},
		base:   "https://api.resend.com",
	}
}

func (r *ResendSender) Send(ctx context.Context, to, subject, html string) error {
	body, err := json.Marshal(map[string]any{
		"from": r.from, "to": []string{to}, "subject": subject, "html": html,
	})
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.base+"/emails", bytes.NewReader(body))
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := r.client.Do(req)
	if err != nil {
		return apperrors.ErrServiceUnavailable.Wrap(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		return apperrors.ErrServiceUnavailable.Wrap(fmt.Errorf("resend returned %d", res.StatusCode))
	}
	return nil
}
