package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestFrontendURLToModel(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name  string
		prior types.String
		api   *string
		want  types.String
	}{
		{"nil reads as null", types.StringValue("https://login.example.com"), nil, types.StringNull()},
		{"empty reads as null", types.StringNull(), str(""), types.StringNull()},
		{"no prior takes the API value", types.StringNull(), str("https://login.example.com"), types.StringValue("https://login.example.com")},
		{"keeps prior case", types.StringValue("https://Login.Example.com"), str("https://login.example.com"), types.StringValue("https://Login.Example.com")},
		{"keeps prior trailing slash", types.StringValue("https://login.example.com/"), str("https://login.example.com"), types.StringValue("https://login.example.com/")},
		{"different value wins", types.StringValue("https://old.example.com"), str("https://new.example.com"), types.StringValue("https://new.example.com")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := frontendURLToModel(tc.prior, tc.api); !got.Equal(tc.want) {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestSameFrontendURL(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://login.example.com", "https://login.example.com", true},
		{"https://Login.example.com", "https://login.example.com", true},
		{"https://login.example.com/", "https://login.example.com", true},
		{"https://login.example.com", "https://other.example.com", false},
	}
	for _, tc := range cases {
		if got := sameFrontendURL(tc.a, tc.b); got != tc.want {
			t.Errorf("sameFrontendURL(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
