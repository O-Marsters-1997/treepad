package batch

// PR is a pull request as `gh pr list` reports it — plain data, no gh
// behaviour. It lives here rather than in internal/gh so batch's exported
// predicates (ReadyToMaterialise, LinkArgs) can take PR state as an argument:
// an exported signature can't name a type from an internal package.
type PR struct {
	Number      int
	HeadRefName string
	BaseRefName string
	State       string // OPEN | MERGED | CLOSED
	URL         string
}
