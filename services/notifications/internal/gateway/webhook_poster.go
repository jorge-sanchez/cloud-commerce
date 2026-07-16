package gateway

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/notifications/internal/service"
)

// HTTPWebhookPoster delivers signed webhook payloads (issue #44).
type HTTPWebhookPoster struct {
	client *http.Client
}

var _ service.WebhookPoster = (*HTTPWebhookPoster)(nil)

// NewHTTPWebhookPoster constructs the poster.
func NewHTTPWebhookPoster() *HTTPWebhookPoster {
	return &HTTPWebhookPoster{client: &http.Client{Timeout: 5 * time.Second}}
}

// Deliver POSTs the envelope with X-CC-Signature: hex HMAC-SHA256(secret, body).
func (p *HTTPWebhookPoster) Deliver(ctx context.Context, url, secret string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CC-Signature", hex.EncodeToString(mac.Sum(nil)))

	res, err := p.client.Do(req)
	if err != nil {
		return apperrors.ErrServiceUnavailable.Wrap(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		return apperrors.ErrServiceUnavailable.Wrap(fmt.Errorf("endpoint returned %d", res.StatusCode))
	}
	return nil
}
