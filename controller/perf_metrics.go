package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func GetPerfMetricsSummary(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	modelGroups := getPerfMetricsModelGroups(c)
	writePerformanceQuery(c, performanceQueryScope{Endpoint: "summary", Hours: hours, Models: modelGroups}, func() (any, error) {
		return perfmetrics.QuerySummaryAll(hours, modelGroups)
	})
}

func GetPerfMetrics(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	activeGroups := getPerfMetricsModelGroups(c)[modelName]
	groupFilter := c.Query("group")
	writePerformanceQuery(c, performanceQueryScope{
		Endpoint: "detail", Hours: hours, Model: modelName, Group: groupFilter,
		Models: map[string][]string{modelName: activeGroups},
	}, func() (any, error) {
		result, err := perfmetrics.Query(perfmetrics.QueryParams{Model: modelName, Group: groupFilter, Hours: hours})
		if err != nil {
			return nil, err
		}
		result.Groups = lo.Filter(result.Groups, func(group perfmetrics.GroupResult, _ int) bool {
			return common.StringsContains(activeGroups, group.Group)
		})
		return result, nil
	})
}

// Use the same current model memberships and viewer permissions as pricing.
// A group can still exist globally after it has been removed from one model.
func getPerfMetricsModelGroups(c *gin.Context) map[string][]string {
	userGroup := ""
	if userID, exists := c.Get("id"); exists {
		if user, err := model.GetUserCache(userID.(int)); err == nil {
			userGroup = user.Group
		}
	}
	return service.GetUserPerformanceModelGroups(userGroup)
}
