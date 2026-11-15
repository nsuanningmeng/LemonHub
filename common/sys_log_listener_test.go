package common

import (
	"bytes"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestStartupBannerUsesEffectiveHTTPListener(t *testing.T) {
	for _, tt := range []struct {
		name, bind, host, url, label string
	}{
		{name: "HOST IPv6", host: "::1", url: "http://[::1]:3000/", label: "Local:"},
		{name: "BIND_ADDRESS takes precedence", bind: "127.0.0.1", host: "192.0.2.10", url: "http://127.0.0.1:3000/", label: "Local:"},
		{name: "hostname", host: "gateway.example", url: "http://gateway.example:3000/", label: "Network:"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BIND_ADDRESS", tt.bind)
			t.Setenv("HOST", tt.host)
			var output bytes.Buffer
			original := gin.DefaultWriter
			gin.DefaultWriter = &output
			t.Cleanup(func() { gin.DefaultWriter = original })
			LogStartupSuccess(time.Now(), "3000")
			assert.Contains(t, output.String(), tt.url)
			assert.Contains(t, output.String(), tt.label)
			assert.NotContains(t, output.String(), "http://localhost:")
		})
	}
}
