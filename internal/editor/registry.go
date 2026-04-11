package editor

import (
	"fmt"
	"sort"
)

type factory func() Adapter

var registry = map[string]factory{}

// Register adds an adapter constructor under a stable name.
// Called from each adapter package's init() function.
func Register(name string, f factory) {
	registry[name] = f
}

// New returns an Adapter for the given name, or an error listing available adapters.
func New(name string) (Adapter, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown editor %q — available: %v", name, Available())
	}
	return f(), nil
}

// Available returns a sorted list of registered editor names.
func Available() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
