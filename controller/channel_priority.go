package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetChannelPriorityOrder(c *gin.Context) {
	order, err := model.GetChannelPriorityOrder()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": order})
}

func ReorderChannelPriority(c *gin.Context) {
	var move model.ChannelPriorityMove
	if err := common.DecodeJson(c.Request.Body, &move); err != nil {
		common.ApiError(c, model.ErrChannelPriorityInvalid)
		return
	}
	order, err := model.ReorderChannelPriority(move)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !move.Preview && len(order.Changes) > 0 {
		model.InitChannelCache()
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": order})
}
