package skills

import (
	"io/fs"
	"strings"
	"testing"
)

// allowedFrontmatterKeys is the six-field subset the Agent Skills spec and the
// claude.ai / Skills API uploader accept. Any other top-level key is a hard
// upload error, not a warning, so a skill that only works locally is a bug.
var allowedFrontmatterKeys = map[string]bool{
	"name":          true,
	"description":   true,
	"license":       true,
	"compatibility": true,
	"metadata":      true,
	"allowed-tools": true,
}

// frontmatter extracts the flat top-level keys from a SKILL.md's YAML
// frontmatter. Only top-level keys are needed here, so this reads them
// line-by-line rather than pulling in a YAML dependency for one check.
func frontmatter(t *testing.T, content string) map[string]string {
	t.Helper()
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		t.Fatalf("SKILL.md missing --- frontmatter delimiters")
	}
	fields := map[string]string{}
	for line := range strings.SplitSeq(parts[1], "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue // indented: a continuation/nested value, not a new key
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return fields
}

func TestEmbeddedSkillsConformToSpec(t *testing.T) {
	names, err := Names()
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("no embedded skills found")
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			sub, err := Open(name)
			if err != nil {
				t.Fatalf("Open(%q): %v", name, err)
			}
			raw, err := fs.ReadFile(sub, "SKILL.md")
			if err != nil {
				t.Fatalf("read SKILL.md: %v", err)
			}
			fm := frontmatter(t, string(raw))

			for key := range fm {
				if !allowedFrontmatterKeys[key] {
					t.Errorf("frontmatter key %q is not one of the six spec-allowed fields", key)
				}
			}
			if fm["name"] != name {
				t.Errorf("frontmatter name = %q, want %q (must match directory name)", fm["name"], name)
			}
			if fm["description"] == "" {
				t.Error("frontmatter description is empty")
			}
			if len(fm["description"]) > 1024 {
				t.Errorf("frontmatter description is %d chars, spec limit is 1024", len(fm["description"]))
			}
		})
	}
}
