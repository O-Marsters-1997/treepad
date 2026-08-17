package batch

import (
	"strings"
	"testing"
)

func TestResolveTicket(t *testing.T) {
	tests := []struct {
		name          string
		ticketURLTmpl string
		input         string
		wantURL       string
		wantRef       string
		wantErr       string
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
			name:          "URL wins even when ticket_url points at a different tracker",
			ticketURLTmpl: "https://github.com/me/other/issues/{{.Ref}}",
			input:         "https://linear.app/acme/issue/ENG-123/silent-refresh",
			wantURL:       "https://linear.app/acme/issue/ENG-123/silent-refresh",
			wantRef:       "silent-refresh",
		},
		{
			name:    "URL with trailing slash",
			input:   "https://github.com/me/treepad/issues/42/",
			wantURL: "https://github.com/me/treepad/issues/42/",
			wantRef: "42",
		},
		{
			name:          "bare Ref rendered through ticket_url",
			ticketURLTmpl: "https://linear.app/acme/issue/{{.Ref}}",
			input:         "ENG-123",
			wantURL:       "https://linear.app/acme/issue/ENG-123",
			wantRef:       "ENG-123",
		},
		{
			name:    "bare Ref with no ticket_url errors naming both fixes",
			input:   "ENG-123",
			wantErr: `no ticket_url configured: cannot resolve "ENG-123"`,
		},
		{
			name:          "malformed ticket_url template",
			ticketURLTmpl: "{{.Ref",
			input:         "ENG-123",
			wantErr:       "parse ticket_url template",
		},
		{
			name:          "ticket_url template referencing an unknown field",
			ticketURLTmpl: "{{.Bogus}}",
			input:         "ENG-123",
			wantErr:       "execute ticket_url template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, ref, err := ResolveTicket(tt.ticketURLTmpl, tt.input)

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

func TestDeriveBranch(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		ref    string
		want   string
	}{
		{name: "prefix with bare ref", prefix: "feat/", ref: "ENG-123", want: "feat/eng-123"},
		{name: "no prefix", prefix: "", ref: "42", want: "42"},
		{name: "prefix with title slug ref", prefix: "feat/", ref: "silent-refresh", want: "feat/silent-refresh"},
		{name: "duplicate refs derive the same branch", prefix: "feat/", ref: "ENG-123", want: "feat/eng-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeriveBranch(tt.prefix, tt.ref); got != tt.want {
				t.Errorf("DeriveBranch(%q, %q) = %q, want %q", tt.prefix, tt.ref, got, tt.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	t.Run("two-Chain Manifest yields correct branch and base for every member", func(t *testing.T) {
		m := Manifest{
			Name:         "silent-refresh",
			BranchPrefix: "feat/",
			Base:         "main",
			Chains: []Chain{
				{Tickets: []string{"ENG-12", "ENG-13"}},
				{Tickets: []string{"ENG-14"}},
			},
		}
		tmpl := "https://linear.app/acme/issue/{{.Ref}}"

		chains, err := Resolve(m, tmpl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(chains) != 2 {
			t.Fatalf("len(chains) = %d, want 2", len(chains))
		}

		chain0 := chains[0]
		if len(chain0) != 2 {
			t.Fatalf("len(chains[0]) = %d, want 2", len(chain0))
		}
		want0 := Member{
			Ticket: "ENG-12", Ref: "ENG-12", TicketURL: "https://linear.app/acme/issue/ENG-12",
			Branch: "feat/eng-12", Base: "main", Batch: "silent-refresh", Chain: 0, Position: 0,
		}
		if chain0[0] != want0 {
			t.Errorf("chains[0][0] = %+v, want %+v", chain0[0], want0)
		}
		want1 := Member{
			Ticket: "ENG-13", Ref: "ENG-13", TicketURL: "https://linear.app/acme/issue/ENG-13",
			Branch: "feat/eng-13", Base: "feat/eng-12", Batch: "silent-refresh", Chain: 0, Position: 1,
		}
		if chain0[1] != want1 {
			t.Errorf("chains[0][1] = %+v, want %+v", chain0[1], want1)
		}

		chain1 := chains[1]
		if len(chain1) != 1 {
			t.Fatalf("len(chains[1]) = %d, want 1", len(chain1))
		}
		want2 := Member{
			Ticket: "ENG-14", Ref: "ENG-14", TicketURL: "https://linear.app/acme/issue/ENG-14",
			Branch: "feat/eng-14", Base: "main", Batch: "silent-refresh", Chain: 1, Position: 0,
		}
		if chain1[0] != want2 {
			t.Errorf("chains[1][0] = %+v, want %+v", chain1[0], want2)
		}
	})

	t.Run("a ticket that fails to resolve errors naming both fixes", func(t *testing.T) {
		m := Manifest{
			BranchPrefix: "feat/",
			Base:         "main",
			Chains:       []Chain{{Tickets: []string{"ENG-12"}}},
		}
		_, err := Resolve(m, "")
		if err == nil || !strings.Contains(err.Error(), "no ticket_url configured") {
			t.Fatalf("got error %v, want no ticket_url configured error", err)
		}
	})
}
