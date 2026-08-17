package fromspec

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"strings"
	"text/template"

	"github.com/O-Marsters-1997/treepad/internal/config"
)

const noTicketURLHint = "set [from_spec] ticket_url in .treepad.toml, or pass the full ticket URL."

// resolveTicket turns user input into a Ticket URL and the Ref it carries.
// Input matching http(s):// is used verbatim; anything else is a Ref rendered
// through cfg.TicketURL.
func resolveTicket(cfg config.FromSpecConfig, input string) (ticketURL, ref string, err error) {
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return input, refOfURL(input), nil
	}

	if cfg.TicketURL == "" {
		return "", "", fmt.Errorf("no ticket_url configured: cannot resolve %q.\n  %s", input, noTicketURLHint)
	}

	rendered, err := renderTicketURL(cfg.TicketURL, input)
	if err != nil {
		return "", "", err
	}
	return rendered, input, nil
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

// specCitation is the ## Spec body: a citation, not the Spec. See ADR 0001.
func specCitation(ticketURL string) string {
	return "Read the ticket at:\n" + ticketURL
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
