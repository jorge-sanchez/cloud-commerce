package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
)

var errWebhookURL = errors.New("webhook url must be https")

// WebhookEndpoint is a merchant-registered delivery target. Secret signs
// every delivery (X-CC-Signature: hex HMAC-SHA256 of the body).
type WebhookEndpoint struct {
	ID        string
	TenantID  string
	URL       string
	Secret    string
	Active    bool
	CreatedAt time.Time
}

// WebhookRepo is the persistence port for endpoints and delivery dedupe.
type WebhookRepo interface {
	SaveEndpoint(ctx context.Context, e *WebhookEndpoint) (*WebhookEndpoint, error)
	ListEndpoints(ctx context.Context, tenantID string) ([]*WebhookEndpoint, error)
	// ListActiveEndpoints is the fan-out read.
	ListActiveEndpoints(ctx context.Context, tenantID string) ([]*WebhookEndpoint, error)
	// DeleteEndpoint is tenant-scoped; unknown is apperrors.ErrNotFound.
	DeleteEndpoint(ctx context.Context, tenantID, id string) error
	// AlreadyDelivered reports whether this endpoint got this event.
	AlreadyDelivered(ctx context.Context, eventID, endpointID string) (bool, error)
	// MarkDelivered records a completed delivery.
	MarkDelivered(ctx context.Context, eventID, endpointID string) error
}

// WebhookPoster delivers one signed payload, implemented in gateway.
type WebhookPoster interface {
	Deliver(ctx context.Context, url, secret string, body []byte) error
}

// WebhookService manages endpoints (owner-only) and fans events out.
type WebhookService interface {
	Register(ctx context.Context, tenantID, actorRole, url string) (*WebhookEndpoint, error)
	List(ctx context.Context, tenantID, actorRole string) ([]*WebhookEndpoint, error)
	Remove(ctx context.Context, tenantID, actorRole, id string) error
	// FanOut posts the envelope to every active endpoint, deduped per
	// endpoint; any failed delivery returns an error so the broker
	// redelivers (succeeded endpoints are already marked and skip).
	FanOut(ctx context.Context, env events.Envelope, rawEnvelope []byte) error
}

type webhookService struct {
	repo   WebhookRepo   // required
	poster WebhookPoster // required
}

// NewWebhookService constructs the webhook application service.
func NewWebhookService(repo WebhookRepo, poster WebhookPoster) WebhookService {
	return &webhookService{repo: repo, poster: poster}
}

func (s *webhookService) Register(ctx context.Context, tenantID, actorRole, url string) (*WebhookEndpoint, error) {
	if actorRole != "owner" {
		return nil, apperrors.ErrForbidden
	}
	if !strings.HasPrefix(url, "https://") {
		return nil, apperrors.ErrValidation.Wrap(errWebhookURL)
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return s.repo.SaveEndpoint(ctx, &WebhookEndpoint{
		TenantID: tenantID, URL: url, Secret: "whsec_cc_" + hex.EncodeToString(raw), Active: true,
	})
}

func (s *webhookService) List(ctx context.Context, tenantID, actorRole string) ([]*WebhookEndpoint, error) {
	if actorRole != "owner" {
		return nil, apperrors.ErrForbidden
	}
	return s.repo.ListEndpoints(ctx, tenantID)
}

func (s *webhookService) Remove(ctx context.Context, tenantID, actorRole, id string) error {
	if actorRole != "owner" {
		return apperrors.ErrForbidden
	}
	return s.repo.DeleteEndpoint(ctx, tenantID, id)
}

func (s *webhookService) FanOut(ctx context.Context, env events.Envelope, rawEnvelope []byte) error {
	endpoints, err := s.repo.ListActiveEndpoints(ctx, env.TenantID)
	if err != nil {
		return err
	}
	for _, e := range endpoints {
		done, err := s.repo.AlreadyDelivered(ctx, env.ID, e.ID)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		// Deliver then mark: a crash in between means a rare duplicate,
		// which beats a lost delivery — consumers dedupe on envelope id.
		if err := s.poster.Deliver(ctx, e.URL, e.Secret, rawEnvelope); err != nil {
			return err // broker retries; marked endpoints skip next time
		}
		if err := s.repo.MarkDelivered(ctx, env.ID, e.ID); err != nil {
			return err
		}
	}
	return nil
}
