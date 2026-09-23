package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

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

	result, err := perfmetrics.QuerySummaryAll(hours, getPerfMetricsModelGroups(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
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

	result, err := perfmetrics.Query(perfmetrics.QueryParams{
		Model: modelName,
		Group: c.Query("group"),
		Hours: hours,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	activeGroups := getPerfMetricsModelGroups(c)[modelName]
	result.Groups = lo.Filter(result.Groups, func(group perfmetrics.GroupResult, _ int) bool {
		return common.StringsContains(activeGroups, group.Group)
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
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
	usableGroups := service.GetUserUsableGroups(userGroup)
	activeRatios := ratio_setting.GetGroupRatioCopy()
	modelGroups := make(map[string][]string)
	for _, pricing := range model.GetPricing() {
		for _, group := range pricing.EnableGroup {
			if group == "" || group == "auto" {
				continue
			}
			if _, ok := usableGroups[group]; !ok {
				continue
			}
			if _, ok := activeRatios[group]; !ok {
				continue
			}
			modelGroups[pricing.ModelName] = append(modelGroups[pricing.ModelName], group)
		}
	}
	return modelGroups
}
