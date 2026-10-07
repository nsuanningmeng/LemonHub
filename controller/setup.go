package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type Setup struct {
	Status       bool   `json:"status"`
	RootInit     bool   `json:"root_init"`
	DatabaseType string `json:"database_type"`
}

type SetupRequest struct {
	Username           string `json:"username"`
	Password           string `json:"password"`
	ConfirmPassword    string `json:"confirmPassword"`
	SelfUseModeEnabled bool   `json:"SelfUseModeEnabled"`
	DemoSiteEnabled    bool   `json:"DemoSiteEnabled"`
}

func GetSetup(c *gin.Context) {
	persisted, err := model.GetSetup()
	if err != nil {
		common.ApiErrorMsg(c, "读取初始化状态失败")
		return
	}
	setup := Setup{Status: persisted != nil}
	if persisted == nil {
		rootExists, err := model.RootUserExists()
		if err != nil {
			common.ApiErrorMsg(c, "读取初始化状态失败")
			return
		}
		setup.RootInit = rootExists
		setup.Status = rootExists
		setup.DatabaseType = string(common.MainDatabaseType())
	}
	c.JSON(200, gin.H{"success": true, "data": setup})
}

func PostSetup(c *gin.Context) {
	// The process flag can be stale on another node. The database is always the
	// authority; failures are not equivalent to an uninitialized installation.
	persisted, err := model.GetSetup()
	if err != nil {
		common.ApiErrorMsg(c, "读取初始化状态失败")
		return
	}
	if persisted != nil {
		common.ApiErrorMsg(c, "系统已经初始化完成")
		return
	}
	rootExists, err := model.RootUserExists()
	if err != nil {
		common.ApiErrorMsg(c, "读取初始化状态失败")
		return
	}
	if rootExists {
		common.ApiErrorMsg(c, "系统已经初始化完成")
		return
	}

	var req SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "请求参数有误")
		return
	}
	if len(req.Username) > 12 {
		common.ApiErrorMsg(c, "用户名长度不能超过12个字符")
		return
	}
	if req.Password != req.ConfirmPassword {
		common.ApiErrorMsg(c, "两次输入的密码不一致")
		return
	}
	if len(req.Password) < 8 {
		common.ApiErrorMsg(c, "密码长度至少为8个字符")
		return
	}
	hashedPassword, err := common.Password2Hash(req.Password)
	if err != nil {
		common.ApiErrorMsg(c, "系统初始化失败")
		return
	}
	// Repeat both checks in the transaction that claims the marker. No root or
	// mode becomes durable until all parts of setup commit successfully.
	if err := model.InitializeSetup(req.Username, hashedPassword, req.SelfUseModeEnabled, req.DemoSiteEnabled); err != nil {
		if errors.Is(err, model.ErrSetupAlreadyInitialized) {
			common.ApiErrorMsg(c, "系统已经初始化完成")
			return
		}
		common.ApiErrorMsg(c, "系统初始化失败")
		return
	}
	c.JSON(200, gin.H{"success": true, "message": "系统初始化成功"})
}
