package slug

import "testing"

func TestSlug(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"treepad", "treepad"},
		{"My.Repo_Name", "my-repo-name"},
		{"feature/my-branch", "feature-my-branch"},
		{"UPPER", "upper"},
		{"hello world", "hello-world"},
		{"--leading-hyphens--", "leading-hyphens"},
		{"123abc", "123abc"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := Slug(tc.input)
			if got != tc.want {
				t.Errorf("Slug(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
