package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const siteAnalyticsPath = "/api/site-analytics"

func TestSiteAnalyticsReportRequiresAdminAndValidDateRange(t *testing.T) {
	engine := setupLoginOriginRouter(t)
	for _, user := range []model.User{
		{Username: "traffic-member", AffCode: "traffic-member", Role: common.RoleCommonUser},
		{Username: "traffic-admin", AffCode: "traffic-admin", Role: common.RoleAdminUser},
	} {
		user.Status = common.UserStatusEnabled
		user.SetAccessToken(user.Username + "-pat")
		require.NoError(t, model.DB.Create(&user).Error)
	}

	for _, tc := range []struct {
		name, token, query string
		wantStatus         int
	}{
		{"anonymous", "", "?days=7", http.StatusUnauthorized},
		{"member", "traffic-member-pat", "?days=7", http.StatusForbidden},
		{"zero days", "traffic-admin-pat", "?days=0", http.StatusBadRequest},
		{"too many days", "traffic-admin-pat", "?days=91", http.StatusBadRequest},
		{"non-numeric days", "traffic-admin-pat", "?days=week", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, siteAnalyticsPath+tc.query, nil)
			if tc.token != "" {
				request.Header.Set("Authorization", "Bearer "+tc.token)
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			assert.Equal(t, tc.wantStatus, response.Code, response.Body.String())
		})
	}
}

func TestSiteAnalyticsReusesBrowserCookieAndReportsVisitsToAdmin(t *testing.T) {
	engine := setupLoginOriginRouter(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SiteTrafficVisit{}))
	admin := model.User{
		Username: "traffic-admin", AffCode: "traffic-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled,
	}
	admin.SetAccessToken("site-analytics-report-pat")
	require.NoError(t, model.DB.Create(&admin).Error)

	visitPath := siteAnalyticsPath + "/visit"
	var cookie *http.Cookie
	for i := 0; i < 2; i++ {
		request := httptest.NewRequest(http.MethodPost, "https://panel.example"+visitPath, nil)
		request.Header.Set("X-Site-Visit", "1")
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
		assert.Empty(t, response.Body.String())
		if i == 0 {
			cookies := response.Result().Cookies()
			require.Len(t, cookies, 1)
			cookie = cookies[0]
			assert.Equal(t, visitPath, cookie.Path)
			assert.Empty(t, cookie.Domain)
			assert.True(t, cookie.HttpOnly)
			assert.True(t, cookie.Secure)
		}
	}

	request := httptest.NewRequest(http.MethodGet, siteAnalyticsPath, nil)
	request.Header.Set("Authorization", "Bearer "+admin.GetAccessToken())
	response := httptest.NewRecorder()
	firstPossibleDay := time.Now().UTC().Format(time.DateOnly)
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
	var report struct {
		Success bool `json:"success"`
		Data    struct {
			Summary map[string]int `json:"summary"`
			Daily   []struct {
				Date      string `json:"date"`
				PageViews int    `json:"page_views"`
			} `json:"daily"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &report))
	require.True(t, report.Success, response.Body.String())
	assert.Equal(t, 2, report.Data.Summary["page_views"])
	assert.Equal(t, 1, report.Data.Summary["unique_visitors"])
	assert.Equal(t, 1, report.Data.Summary["new_visitors"])
	require.Len(t, report.Data.Daily, 7)
	lastDay := report.Data.Daily[len(report.Data.Daily)-1].Date
	assert.Contains(t, []string{firstPossibleDay, time.Now().UTC().Format(time.DateOnly)}, lastDay)
	var views int
	for _, day := range report.Data.Daily {
		views += day.PageViews
	}
	assert.Equal(t, 2, views)
}

func TestSiteAnalyticsSeparatesMainAndSubSiteVisitsByRequestHost(t *testing.T) {
	setupLoginOriginRouter(t)
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.Site{}, &model.SiteDomain{}, &model.SiteTrafficVisit{}))
	const mainHost = "portal.example"
	const tenantHost = "partner.example"
	site := model.Site{Name: "Partner"}
	require.NoError(t, db.Create(&site).Error)
	domain := model.SiteDomain{SiteId: site.Id, Domain: tenantHost}
	require.NoError(t, db.Create(&domain).Error)
	t.Cleanup(func() {
		assert.NoError(t, db.Delete(&domain).Error)
		assert.NoError(t, db.Delete(&site).Error)
		assert.NoError(t, model.ReloadSiteCache())
	})
	require.NoError(t, model.ReloadSiteCache())

	admin := model.User{
		Username: "platform-admin", AffCode: "traffic-sites", Role: common.RoleAdminUser, Status: common.UserStatusEnabled,
	}
	admin.SetAccessToken("traffic-sites-admin-pat")
	require.NoError(t, db.Create(&admin).Error)
	// Register the real routes after the host resolver, as in main.go.
	engine := gin.New()
	engine.Use(middleware.SiteResolver())
	SetApiRouter(engine)

	for _, host := range []string{mainHost, tenantHost, tenantHost} {
		request := httptest.NewRequest(http.MethodPost, "https://"+host+siteAnalyticsPath+"/visit", nil)
		request.Header.Set("X-Site-Visit", "1")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
	}
	for _, tc := range []struct {
		host  string
		count int64
	}{{mainHost, 1}, {tenantHost, 2}} {
		t.Run(tc.host, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://"+tc.host+siteAnalyticsPath, nil)
			request.Header.Set("Authorization", "Bearer "+admin.GetAccessToken())
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var report struct {
				Success bool                    `json:"success"`
				Data    model.SiteTrafficReport `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &report))
			require.True(t, report.Success, response.Body.String())
			assert.Equal(t, model.SiteTrafficCounts{
				PageViews: tc.count, UniqueVisitors: tc.count, NewVisitors: tc.count,
			}, report.Data.Summary)
		})
	}
}

func TestSiteAnalyticsSkipsOptedOutAndInvalidVisits(t *testing.T) {
	engine := setupLoginOriginRouter(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SiteTrafficVisit{}))
	for _, tc := range []struct {
		name, visitHeader, fetchSite, dnt, gpc, body string
		wantStatus                                   int
	}{
		{name: "missing visit header", wantStatus: http.StatusForbidden},
		{name: "cross-site request", visitHeader: "1", fetchSite: "cross-site", wantStatus: http.StatusForbidden},
		{name: "non-empty body", visitHeader: "1", body: "unexpected", wantStatus: http.StatusBadRequest},
		{name: "do not track", visitHeader: "1", dnt: "1", wantStatus: http.StatusNoContent},
		{name: "global privacy control", visitHeader: "1", gpc: "1", wantStatus: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, siteAnalyticsPath+"/visit", strings.NewReader(tc.body))
			request.Header.Set("X-Site-Visit", tc.visitHeader)
			request.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			request.Header.Set("DNT", tc.dnt)
			request.Header.Set("Sec-GPC", tc.gpc)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			assert.Equal(t, tc.wantStatus, response.Code)
			assert.Empty(t, response.Result().Cookies())
		})
	}
	var count int64
	require.NoError(t, model.DB.Model(&model.SiteTrafficVisit{}).Count(&count).Error)
	assert.Zero(t, count)
}
