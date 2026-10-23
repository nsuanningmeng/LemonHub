package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// GetModelPerformance returns the aggregate health of the models and groups
// available to the authenticated user, independent of pricing page visibility.
// The default 24-hour window matches the model square.
func GetModelPerformance(c *gin.Context) {
	hours := 24
	if values, supplied := c.Request.URL.Query()["hours"]; supplied {
		parsed, err := strconv.Atoi(values[0])
		if len(values) != 1 || err != nil || parsed < 1 || parsed > 720 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"code":    "INVALID_HOURS",
				"message": "hours must be an integer between 1 and 720",
			})
			return
		}
		hours = parsed
	}

	// The group comes from UserAuth, never from a caller-supplied user ID.
	modelGroups := service.GetUserPerformanceModelGroups(c.GetString("group"))
	modelFilter, groupFilter := c.Query("model"), c.Query("group")
	for modelName, groups := range modelGroups {
		if modelFilter != "" && modelName != modelFilter {
			delete(modelGroups, modelName)
			continue
		}
		if groupFilter == "" {
			continue
		}
		if !common.StringsContains(groups, groupFilter) {
			delete(modelGroups, modelName)
			continue
		}
		modelGroups[modelName] = []string{groupFilter}
	}

	writePerformanceQuery(c, performanceQueryScope{Endpoint: "models", Hours: hours, Models: modelGroups}, func() (any, error) {
		return perfmetrics.QueryModels(hours, modelGroups)
	})
}
