package controller

import (
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetUserUsageActivity(c *gin.Context) {
	userID := c.GetInt("id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Authentication required"})
		return
	}
	zone := c.DefaultQuery("timezone", "UTC")
	location, err := time.LoadLocation(zone)
	if err != nil || zone == "" || zone == "Local" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid IANA timezone"})
		return
	}
	activity, err := model.GetUserUsageActivity(c.Request.Context(), userID, location, time.Now())
	if err != nil {
		common.SysError("failed to load usage activity: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Unable to load usage activity"})
		return
	}
	common.ApiSuccess(c, activity)
}
