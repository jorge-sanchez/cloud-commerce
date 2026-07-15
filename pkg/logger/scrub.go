package logger

import "sync"

// scrubFields lists field names whose values must never appear in log output.
// This policy protects against accidental logging of request/response bodies
// and authentication credentials.
//
// Scrub policy:
//   - "body", "request_body", "response_body" — HTTP payload content
//   - "password", "token" — authentication / secret material
//
// Any zap field whose key matches an entry in this list is replaced with
// RedactedValue before the entry is forwarded to the underlying Core.
//
// Services can extend this list at startup via RegisterScrubFields — do NOT
// add service-specific fields here; add them to the service's own init call.
var scrubFields = map[string]struct{}{
	"body":          {},
	"request_body":  {},
	"response_body": {},
	"password":      {},
	"token":         {},
}

// scrubMu protects scrubFields against concurrent reads and writes.
// RegisterScrubFields may be called from init() functions that run in
// parallel with package initialisation in other goroutines.
var scrubMu sync.RWMutex

// RegisterScrubFields adds service-specific sensitive field names to the
// scrub list. Call this once at service startup (e.g. from an init()
// function or main()) before any log entries are written.
//
// Example:
//
//	func init() {
//	    logger.RegisterScrubFields("apns_certificate", "gcm_server_key")
//	}
func RegisterScrubFields(fields ...string) {
	scrubMu.Lock()
	defer scrubMu.Unlock()
	for _, f := range fields {
		scrubFields[f] = struct{}{}
	}
}

// RedactedValue is the placeholder written in place of a scrubbed field value.
const RedactedValue = "[REDACTED]"
