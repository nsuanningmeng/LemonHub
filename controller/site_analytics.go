package controller

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const siteVisitorCookie = "site_visitor"

func RecordSiteVisit(c *gin.Context) {
	if c.GetHeader("DNT") == "1" || c.GetHeader("Sec-GPC") == "1" {
		c.Status(http.StatusNoContent)
		return
	}
	// Browsers cannot send this custom header with a cross-origin form. A
	// same-origin fetch also stays valid behind a TLS-terminating proxy.
	fetchSite := c.GetHeader("Sec-Fetch-Site")
	if c.GetHeader("X-Site-Visit") != "1" || (fetchSite != "" && fetchSite != "same-origin") {
		c.Status(http.StatusForbidden)
		return
	}
	if c.Request.ContentLength != 0 {
		c.Status(http.StatusBadRequest)
		return
	}

	rawCookie, _ := c.Cookie(siteVisitorCookie)
	visitorID, parseErr := uuid.Parse(rawCookie)
	if parseErr != nil {
		visitorID = uuid.New()
	}
	hash := hex.EncodeToString(common.Sha256Raw(visitorID[:]))
	if err := model.RecordSitePageView(c.Request.Context(), middleware.GetRequestSiteId(c), hash, time.Now()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("could not record site visit: %v", err))
		c.Status(http.StatusInternalServerError)
		return
	}
	if parseErr != nil {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     siteVisitorCookie,
			Value:    visitorID.String(),
			Path:     c.FullPath(),
			MaxAge:   365 * 24 * 60 * 60,
			HttpOnly: true,
			Secure:   common.SessionCookieSecure || c.Request.TLS != nil,
			SameSite: http.SameSiteLaxMode,
		})
	}
	c.Status(http.StatusNoContent)
}

func GetSiteAnalytics(c *gin.Context) {
	days := 7
	if values, ok := c.GetQueryArray("days"); ok {
		value, err := strconv.Atoi(values[0])
		if err != nil || len(values) != 1 || value < 1 || value > model.MaxSiteTrafficDays {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgInvalidParams)})
			return
		}
		days = value
	}
	report, err := model.GetSiteTraffic(c.Request.Context(), middleware.GetRequestSiteId(c), days, time.Now())
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("could not read site traffic: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgDatabaseError)})
		return
	}
	common.ApiSuccess(c, report)
}
