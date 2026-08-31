package middleware

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const RouteTagKey = "route_tag"
const accessLogPathKey = "access_log_path"

func RouteTag(tag string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(RouteTagKey, tag)
		c.Next()
	}
}

func SetUpLogger(server *gin.Engine) {
	logger := gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var requestID string
		if param.Keys != nil {
			requestID, _ = param.Keys[common.RequestIdKey].(string)
		}
		tag, _ := param.Keys[RouteTagKey].(string)
		if tag == "" {
			tag = "web"
		}
		path, _ := param.Keys[accessLogPathKey].(string)
		if path == "" {
			path = "<unmatched>"
		}
		return fmt.Sprintf("[GIN] %s | %s | %s | %3d | %13v | %15s | %7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			tag,
			requestID,
			param.StatusCode,
			param.Latency,
			param.ClientIP,
			param.Method,
			path,
		)
	})
	server.Use(logger, func(c *gin.Context) {
		c.Next()
		c.Set(accessLogPathKey, c.FullPath())
	})
}
