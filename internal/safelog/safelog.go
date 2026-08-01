// Package safelog provides structured logging with defense-in-depth secret
// redaction. Callers should still avoid logging sensitive values explicitly.
package safelog

import (
	"io"
	"log/slog"
	"sort"
	"strings"
)

const replacement = "[REDACTED]"

// New creates a reusable redactor for the supplied secrets.
func New(secrets ...string) Redactor {
	return Redactor{secrets: usableSecrets(secrets)}
}

// JSONHandler creates a JSON handler that redacts secrets from string and error
// attributes, including attributes nested in groups.
func (r Redactor) JSONHandler(w io.Writer) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{
		ReplaceAttr: r.replaceAttr,
	})
}

// Redactor removes a fixed set of secrets from text and log attributes.
type Redactor struct {
	secrets []string
}

func usableSecrets(secrets []string) []string {
	usable := make([]string, 0, len(secrets))
	seen := make(map[string]struct{}, len(secrets))
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		if _, exists := seen[secret]; exists {
			continue
		}
		seen[secret] = struct{}{}
		usable = append(usable, secret)
	}
	sort.Slice(usable, func(i, j int) bool { return len(usable[i]) > len(usable[j]) })
	return usable
}

func (r Redactor) String(value string) string {
	for _, secret := range r.secrets {
		value = strings.ReplaceAll(value, secret, replacement)
	}
	return value
}

func (r Redactor) replaceAttr(_ []string, attr slog.Attr) slog.Attr {
	value := attr.Value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		attr.Value = slog.StringValue(r.String(value.String()))
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			attr.Value = slog.StringValue(r.String(err.Error()))
		}
	}
	return attr
}
