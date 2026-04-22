package treepad

import (
	"path/filepath"
	"sort"

	"github.com/sahilm/fuzzy"
)

// filterRows returns rows matching query, ordered by best score.
// Branch and path basename are matched independently so that characters from
// one field cannot form a subsequence that spans into the other.
// An empty query returns rows unchanged.
func filterRows(rows []StatusRow, query string) []StatusRow {
	if query == "" {
		return rows
	}

	branches := make([]string, len(rows))
	basenames := make([]string, len(rows))
	for i, r := range rows {
		branches[i] = r.Branch
		basenames[i] = filepath.Base(r.Path)
	}

	// scores maps row index → best match score across both fields.
	scores := make(map[int]int)
	for _, m := range fuzzy.Find(query, branches) {
		scores[m.Index] = m.Score
	}
	for _, m := range fuzzy.Find(query, basenames) {
		if existing, ok := scores[m.Index]; !ok || m.Score > existing {
			scores[m.Index] = m.Score
		}
	}

	type indexScore struct{ idx, score int }
	ranked := make([]indexScore, 0, len(scores))
	for idx, score := range scores {
		ranked = append(ranked, indexScore{idx, score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	out := make([]StatusRow, len(ranked))
	for i, is := range ranked {
		out[i] = rows[is.idx]
	}
	return out
}
