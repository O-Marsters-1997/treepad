package commands

import (
	"reflect"
	"testing"
)

func TestParseTickets(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr string
	}{
		{
			name: "mixed bare refs and URLs",
			raw:  "ENG-12,https://github.com/acme/widgets/issues/14,ENG-19",
			want: []string{"ENG-12", "https://github.com/acme/widgets/issues/14", "ENG-19"},
		},
		{
			name: "whitespace around entries is trimmed",
			raw:  " ENG-12 , ENG-14 ",
			want: []string{"ENG-12", "ENG-14"},
		},
		{
			name: "empty segments are dropped",
			raw:  "ENG-12,,ENG-14,",
			want: []string{"ENG-12", "ENG-14"},
		},
		{
			name:    "all empty errors",
			raw:     " , , ",
			wantErr: "--tickets requires at least one ticket",
		},
		{
			name: "non-numeric refs are accepted",
			raw:  "ENG-123",
			want: []string{"ENG-123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTickets(tt.raw)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("got error %v, want error %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
