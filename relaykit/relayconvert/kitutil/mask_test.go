package kitutil

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSensitiveNetworkAddressesStayPrivate(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"dial tcp [2001:db8::1]:443", "dial tcp [***]:443"},
		{"dial tcp [fe80::1%eth0]:443", "dial tcp [***]:443"},
		{"dial tcp [::ffff:192.168.1.3]:443", "dial tcp [***]:443"},
		{"GET https://[2001:db8::1]:443/private?key=secret", "GET https://[***]:443/***?key=***"},
		{"GET https://192.168.1.3:443/private", "GET https://***.***.***.***:443/***"},
		{"GET https://internal.example.co.uk:8443/private", "GET https://***.***.co.uk:8443/***"},
		{"lookup private.example.com", "lookup ***.***.com"},
	} {
		t.Run(tc.input, func(t *testing.T) { assert.Equal(t, tc.want, MaskSensitiveInfo(tc.input)) })
	}
}
