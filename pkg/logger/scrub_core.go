package logger

import (
	"go.uber.org/zap/zapcore"
)

// scrubCore wraps a zapcore.Core and replaces the values of any fields whose
// key appears in scrubFields with RedactedValue before forwarding to the
// inner core.
type scrubCore struct {
	inner  zapcore.Core
	fields []zapcore.Field
}

// ScrubCore returns a zapcore.Core that redacts sensitive fields before
// passing log entries to inner. Wrap the core returned by zap.NewCore (or
// any other core) with this function to enforce the body-scrub policy.
func ScrubCore(inner zapcore.Core) zapcore.Core {
	return &scrubCore{inner: inner}
}

func (s *scrubCore) Enabled(lvl zapcore.Level) bool {
	return s.inner.Enabled(lvl)
}

func (s *scrubCore) With(fields []zapcore.Field) zapcore.Core {
	return &scrubCore{
		inner:  s.inner.With(redactFields(fields)),
		fields: append(s.fields, fields...),
	}
}

func (s *scrubCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if s.inner.Enabled(entry.Level) {
		return ce.AddCore(entry, s)
	}
	return ce
}

func (s *scrubCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	return s.inner.Write(entry, redactFields(fields))
}

func (s *scrubCore) Sync() error {
	return s.inner.Sync()
}

// redactFields returns a new slice with sensitive field values replaced.
func redactFields(fields []zapcore.Field) []zapcore.Field {
	scrubMu.RLock()
	defer scrubMu.RUnlock()
	out := make([]zapcore.Field, len(fields))
	for i, f := range fields {
		if _, sensitive := scrubFields[f.Key]; sensitive && f.Type == zapcore.StringType {
			f.String = RedactedValue
		}
		out[i] = f
	}
	return out
}
