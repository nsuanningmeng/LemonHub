package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/service/captcha"
	"github.com/gin-gonic/gin"
)

// GetAltchaChallenge serves a fresh proof-of-work challenge for the ALTCHA
// widget. The response is the raw challenge object (not the usual API
// envelope) because that is the shape the widget's challengeurl expects.
func GetAltchaChallenge(c *gin.Context) {
	challenge, err := captcha.CreateAltchaChallenge()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, challenge)
}
