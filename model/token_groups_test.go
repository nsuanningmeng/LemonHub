package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenGetGroups(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single group (legacy)", "vip", []string{"vip"}},
		{"multiple ordered", "vip,default,free", []string{"vip", "default", "free"}},
		{"trims surrounding spaces", " vip , default ", []string{"vip", "default"}},
		{"dedup preserves first occurrence", "vip,vip,default,vip", []string{"vip", "default"}},
		{"strips leading/trailing/empty segments", ",vip,,default,", []string{"vip", "default"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, (&Token{Group: tc.in}).GetGroups())
		})
	}
}

func TestNormalizeGroupString(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"vip", "vip"},
		{"vip,default,free", "vip,default,free"},
		{" vip , default ", "vip,default"},
		{"vip,vip,default", "vip,default"},
		{",vip,,default,", "vip,default"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, NormalizeGroupString(tc.in))
	}
}
