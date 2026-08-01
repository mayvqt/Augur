package safelog

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestJSONHandlerRedactsSecretsFromStringsErrorsAndGroups(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	logger := slog.New(New("discord-secret", "seer-secret").JSONHandler(&output))
	logger.Error(
		"failed with discord-secret",
		"error", errors.New("upstream returned seer-secret"),
		"nested", slog.GroupValue(slog.String("value", "discord-secret")),
	)

	got := output.String()
	for _, secret := range []string{"discord-secret", "seer-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("log contains %q: %s", secret, got)
		}
	}
	if count := strings.Count(got, replacement); count != 3 {
		t.Fatalf("redaction count = %d, want 3: %s", count, got)
	}
}

func TestRedactorIgnoresEmptySecrets(t *testing.T) {
	t.Parallel()
	if got := New("", "  ").String("unchanged"); got != "unchanged" {
		t.Fatalf("String() = %q, want unchanged", got)
	}
}

func TestRedactorDeduplicatesAndOrdersOverlappingSecrets(t *testing.T) {
	t.Parallel()
	redactor := New("secret", "secret-value", "secret")
	if got := redactor.String("secret-value"); got != replacement {
		t.Fatalf("String() = %q, want complete redaction", got)
	}
	if len(redactor.secrets) != 2 {
		t.Fatalf("secret count = %d, want 2", len(redactor.secrets))
	}
}
