package launcher

import (
	"reflect"
	"testing"
)

func TestRender(t *testing.T) {
	data := Data{
		Branch:       "feat/eng-12",
		Slug:         "treepad",
		WorktreePath: "/repos/treepad-feat-eng-12",
		TicketURL:    "https://linear.app/acme/issue/ENG-12",
		Ref:          "ENG-12",
		ActivityFile: "/common/treepad/activity/feat-eng-12.log",
		Batch:        "silent-refresh",
		Chain:        0,
		Position:     1,
	}

	cases := []struct {
		name string
		tmpl string
		want string
	}{
		{"Branch", "{{.Branch}}", "feat/eng-12"},
		{"Slug", "{{.Slug}}", "treepad"},
		{"WorktreePath", "{{.WorktreePath}}", "/repos/treepad-feat-eng-12"},
		{"TicketURL", "{{.TicketURL}}", "https://linear.app/acme/issue/ENG-12"},
		{"Ref", "{{.Ref}}", "ENG-12"},
		{"ActivityFile", "{{.ActivityFile}}", "/common/treepad/activity/feat-eng-12.log"},
		{"Batch", "{{.Batch}}", "silent-refresh"},
		{"Chain", "{{.Chain}}", "0"},
		{"Position", "{{.Position}}", "1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render([]string{tc.tmpl}, data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := []string{tc.want}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Render(%q) = %v, want %v", tc.tmpl, got, want)
			}
		})
	}

	t.Run("renders every element in order", func(t *testing.T) {
		got, err := Render([]string{"claude", "--dangerously-skip-permissions", "{{.TicketURL}}"}, data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"claude", "--dangerously-skip-permissions", "https://linear.app/acme/issue/ENG-12"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Render() = %v, want %v", got, want)
		}
	})

	t.Run("empty template list renders to nil, not an error", func(t *testing.T) {
		got, err := Render(nil, data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("Render(nil) = %v, want nil", got)
		}
	})

	t.Run("an unknown field is a parse error", func(t *testing.T) {
		if _, err := Render([]string{"{{.NotAField}}"}, data); err == nil {
			t.Error("expected an error for an unknown template field, got nil")
		}
	})
}

func TestActivityPath(t *testing.T) {
	got := ActivityPath("/common", "feat/eng-12")
	want := "/common/treepad/activity/feat-eng-12.log"
	if got != want {
		t.Errorf("ActivityPath() = %q, want %q", got, want)
	}
}
