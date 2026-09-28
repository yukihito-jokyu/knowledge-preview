package domain

import (
	"strings"
	"testing"
)

func TestValidPublicID(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		want  bool
	}{
		{name: "32 byte base64url", value: strings.Repeat("A", 43), want: true},
		{name: "too short", value: strings.Repeat("A", 42)},
		{name: "too long", value: strings.Repeat("A", 44)},
		{name: "invalid alphabet", value: strings.Repeat("+", 43)},
		{name: "padding", value: strings.Repeat("A", 42) + "="},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ValidPublicID(test.value); got != test.want {
				t.Fatalf("ValidPublicID(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}
