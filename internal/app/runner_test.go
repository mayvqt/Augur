package app

import (
	"strings"
	"testing"

	"github.com/mayvqt/Augur/internal/seer"
)

func TestValidateSearchResult(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		result  seer.SearchResult
		wantErr string
	}{
		{name: "movie", result: seer.SearchResult{ID: 1, MediaType: "movie"}},
		{name: "tv", result: seer.SearchResult{ID: 1, MediaType: "tv"}},
		{name: "missing id", result: seer.SearchResult{MediaType: "movie"}, wantErr: "missing an ID"},
		{name: "unsupported type", result: seer.SearchResult{ID: 1, MediaType: "music"}, wantErr: "unsupported media type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSearchResult(tt.result)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateSearchResult() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateSearchResult() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
