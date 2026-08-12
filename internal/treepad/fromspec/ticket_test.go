package fromspec

import (
	"strings"
	"testing"

	"treepad/internal/config"
)

func TestResolveTicket(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.FromSpecConfig
		input   string
		wantURL string
		wantRef string
		wantErr string
	}{
		{
			name:    "Linear URL used verbatim, ref is last path segment",
			input:   "https://linear.app/acme/issue/ENG-123/silent-refresh",
			wantURL: "https://linear.app/acme/issue/ENG-123/silent-refresh",
			wantRef: "silent-refresh",
		},
		{
			name:    "GitHub URL used verbatim, ref is the issue number",
			input:   "https://github.com/me/treepad/issues/42",
			wantURL: "https://github.com/me/treepad/issues/42",
			wantRef: "42",
		},
		{
			name:    "URL wins even when ticket_url points at a different tracker",
			cfg:     config.FromSpecConfig{TicketURL: "https://github.com/me/other/issues/{{.Ref}}"},
			input:   "https://linear.app/acme/issue/ENG-123/silent-refresh",
			wantURL: "https://linear.app/acme/issue/ENG-123/silent-refresh",
			wantRef: "silent-refresh",
		},
		{
			name:    "URL with trailing slash",
			input:   "https://github.com/me/treepad/issues/42/",
			wantURL: "https://github.com/me/treepad/issues/42/",
			wantRef: "42",
		},
		{
			name:    "bare Ref rendered through ticket_url",
			cfg:     config.FromSpecConfig{TicketURL: "https://linear.app/acme/issue/{{.Ref}}"},
			input:   "ENG-123",
			wantURL: "https://linear.app/acme/issue/ENG-123",
			wantRef: "ENG-123",
		},
		{
			name:    "bare Ref with no ticket_url errors naming both fixes",
			input:   "ENG-123",
			wantErr: `no ticket_url configured: cannot resolve "ENG-123"`,
		},
		{
			name:    "malformed ticket_url template",
			cfg:     config.FromSpecConfig{TicketURL: "{{.Ref"},
			input:   "ENG-123",
			wantErr: "parse ticket_url template",
		},
		{
			name:    "ticket_url template referencing an unknown field",
			cfg:     config.FromSpecConfig{TicketURL: "{{.Bogus}}"},
			input:   "ENG-123",
			wantErr: "execute ticket_url template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, ref, err := resolveTicket(tt.cfg, tt.input)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got error %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if url != tt.wantURL {
				t.Errorf("url = %q, want %q", url, tt.wantURL)
			}
			if ref != tt.wantRef {
				t.Errorf("ref = %q, want %q", ref, tt.wantRef)
			}
		})
	}
}
