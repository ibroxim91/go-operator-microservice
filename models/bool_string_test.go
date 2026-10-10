package models

import (
	"encoding/json"
	"testing"
)

func TestBoolStringUnmarshalJSON(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"bool true", `true`, true},
		{"bool false", `false`, false},
		{"string true", `"true"`, true},
		{"string True", `"True"`, true},
		{"string false", `"false"`, false},
		{"string 1", `"1"`, true},
		{"string 0", `"0"`, false},
		{"number 1", `1`, true},
		{"number 0", `0`, false},
		{"empty string", `""`, false},
		{"null", `null`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got BoolString
			if err := json.Unmarshal([]byte(tc.input), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Bool() != tc.want {
				t.Fatalf("got %v want %v for input %s", got.Bool(), tc.want, tc.input)
			}
			if IsBronBookable(got) != tc.want {
				t.Fatalf("IsBronBookable got %v want %v", IsBronBookable(got), tc.want)
			}
		})
	}
}
