package batch

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"strings"
	"text/template"

	"github.com/O-Marsters-1997/treepad/internal/slug"
)

const noTicketURLHint = "set [from_spec] ticket_url in .treepad.toml, or pass the full ticket URL."

// Member is a Chain member resolved against config: one Ticket, its Ref, the
// branch it seeds, and the base that branch is created from.
//
// Base at Chain position 0 is Chain.Base, falling back to Manifest.Base; at
// every later position it is the previous member's Branch.
type Member struct {
	Ticket    string `json:"ticket"`
	Ref       string `json:"ref"`
	TicketURL string `json:"ticket_url"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
	Batch     string `json:"batch"`
	Chain     int    `json:"chain"`
	Position  int    `json:"position"`
}

// Resolve is pure: no context.Context, no runner, no filesystem. It turns a
// Manifest's Chains into resolved Members, one slice per Chain.
func Resolve(m Manifest, ticketURLTmpl string) ([][]Member, error) {
	chains := make([][]Member, len(m.Chains))
	for ci, chain := range m.Chains {
		members := make([]Member, len(chain.Tickets))
		base := m.Base
		if chain.Base != "" {
			base = chain.Base
		}
		for pi, ticket := range chain.Tickets {
			ticketURL, ref, err := ResolveTicket(ticketURLTmpl, ticket)
			if err != nil {
				return nil, fmt.Errorf("batch %s, chain %d: %w", m.Name, ci, err)
			}
			branch := DeriveBranch(m.BranchPrefix, ref)
			members[pi] = Member{
				Ticket:    ticket,
				Ref:       ref,
				TicketURL: ticketURL,
				Branch:    branch,
				Base:      base,
				Batch:     m.Name,
				Chain:     ci,
				Position:  pi,
			}
			base = branch
		}
		chains[ci] = members
	}
	return chains, nil
}

// ResolveTicket turns user input into a Ticket URL and the Ref it carries.
// Input matching http(s):// is used verbatim; anything else is a Ref rendered
// through ticketURLTmpl.
func ResolveTicket(ticketURLTmpl, input string) (ticketURL, ref string, err error) {
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return input, refOfURL(input), nil
	}

	if ticketURLTmpl == "" {
		return "", "", fmt.Errorf("no ticket_url configured: cannot resolve %q.\n  %s", input, noTicketURLHint)
	}

	rendered, err := renderTicketURL(ticketURLTmpl, input)
	if err != nil {
		return "", "", err
	}
	return rendered, input, nil
}

// DeriveBranch computes a branch name from prefix + slug(ref). Refs are
// unique within a Tracker, so no collision suffix is needed. It must stay
// deterministic and total: reconcile re-derives every branch name on every
// tick and stores none.
// ponytail: a Linear ref is a title slug (feat/silent-refresh), a GitHub ref
// is bare digits (feat/42) — readability trade-off accepted in ADR 0001.
func DeriveBranch(prefix, ref string) string {
	return prefix + slug.Slug(ref)
}

// refOfURL returns a URL's last non-empty path segment.
// ponytail: purely positional, no Tracker knowledge — yields title slugs on
// Linear and bare numbers on GitHub. Upgrade to per-Tracker parsing only if
// some Tracker's URL shape makes the last segment useless.
func refOfURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return path.Base(strings.TrimRight(u.Path, "/"))
}

func renderTicketURL(tmpl, ref string) (string, error) {
	t, err := template.New("ticket_url").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse ticket_url template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct{ Ref string }{Ref: ref}); err != nil {
		return "", fmt.Errorf("execute ticket_url template: %w", err)
	}
	return buf.String(), nil
}
