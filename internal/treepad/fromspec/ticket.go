package fromspec

// specCitation is the ## Spec body: a citation, not the Spec. See ADR 0001.
func specCitation(ticketURL string) string {
	return "Read the ticket at:\n" + ticketURL
}
