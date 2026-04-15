package treepad

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestServiceCD(t *testing.T) {
	tests := []struct {
		name        string
		branch      string
		wantCD      string
		wantErr     bool
		wantErrText string
	}{
		{
			name:   "cds into existing worktree",
			branch: "feat",
			wantCD: "__TREEPAD_CD__\t/repo/feat\n",
		},
		{
			name:        "branch not found",
			branch:      "missing",
			wantErr:     true,
			wantErrText: "no worktree found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			svc := NewService(fakeRunner{output: twoWorktreePorcelain}, &fakeSyncer{}, nil, &out)

			err := svc.CD(context.Background(), CDInput{Branch: tt.branch})

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrText != "" && !strings.Contains(err.Error(), tt.wantErrText) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErrText)
				}
				if out.Len() > 0 {
					t.Errorf("no output expected on error, got: %q", out.String())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := out.String(); got != tt.wantCD {
				t.Errorf("output = %q, want %q", got, tt.wantCD)
			}
		})
	}
}
