package batch

// ReadyToMaterialise returns the leading prefix of chain eligible for this
// tick: position 0 is always ready; position i is ready when member i-1's
// branch has an open (or merged) pull request. A pushed branch alone is not
// enough — an agent pushing a work-in-progress commit must not unblock the
// layer above against nothing.
//
// An already-materialised member (existing[m.Branch] true) stays ready
// regardless of its parent's current pull request state, so a pull request
// closing after materialisation never un-reports something already created.
func ReadyToMaterialise(chain []Member, existing map[string]bool, prs map[string]PR) []Member {
	ready := make([]Member, 0, len(chain))
	for i, m := range chain {
		if i == 0 || existing[m.Branch] {
			ready = append(ready, m)
			continue
		}
		pr, ok := prs[m.Base]
		if !ok || (pr.State != "OPEN" && pr.State != "MERGED") {
			break
		}
		ready = append(ready, m)
	}
	return ready
}
