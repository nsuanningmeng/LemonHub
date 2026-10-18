package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
)

func RelayPanicRecover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Panic values and request headers can contain credentials or bodies.
				// Keep a correlated stack for diagnosis without formatting either.
				logger.LogError(c, fmt.Sprintf("panic recovered (%T); stacktrace: %s", err, debug.Stack()))
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{
						"message": "An internal server error occurred.",
						"type":    "new_api_panic",
					},
				})
			}
		}()
		c.Next()
	}
}
