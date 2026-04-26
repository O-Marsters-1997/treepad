package treepadtest

import "context"

type Runner struct {
	Calls [][]string
	Err   error
}

func (f *Runner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	entry := append([]string{name}, args...)
	f.Calls = append(f.Calls, entry)
	return nil, f.Err
}
