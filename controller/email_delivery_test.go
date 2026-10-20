package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResendOptionsNeverExposeCredentials(t *testing.T) {
	previous := common.OptionMap
	t.Cleanup(func() { common.OptionMap = previous })
	for _, key := range []string{"", "re_secret_value"} {
		common.OptionMap = map[string]string{"EmailProvider": "resend", "ResendApiKey": key, "ResendFrom": "Sender <sender@example.com>", "ResendKeyConfigured": "spoofed", "MarketingSMTPToken": "marketing-secret"}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		GetOptions(c)
		var response struct {
			Success bool           `json:"success"`
			Data    []model.Option `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		require.True(t, response.Success)
		options := map[string]string{}
		for _, option := range response.Data {
			options[option.Key] = option.Value
		}
		assert.NotContains(t, options, "ResendApiKey")
		assert.NotContains(t, options, "MarketingSMTPToken")
		assert.NotContains(t, w.Body.String(), "re_secret_value")
		assert.NotContains(t, w.Body.String(), "marketing-secret")
		assert.Equal(t, "Sender <sender@example.com>", options["ResendFrom"])
		if key == "" {
			assert.Equal(t, "false", options["ResendKeyConfigured"])
		} else {
			assert.Equal(t, "true", options["ResendKeyConfigured"])
		}
	}
}
