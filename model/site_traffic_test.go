package model

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

func TestSiteTrafficVisitorCounts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "traffic.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	oldDB := DB
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		assert.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&SiteTrafficVisit{}))

	// 01:00 on November 22 at UTC+8 is still November 21 in the report.
	now := time.Date(2026, 11, 22, 1, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	firstDay := now.AddDate(0, 0, -3)
	returning, first, last := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	ctx := t.Context()
	var group errgroup.Group
	for range 2 {
		group.Go(func() error { return RecordSitePageView(ctx, 0, returning, firstDay) })
	}
	require.NoError(t, group.Wait())
	for _, visit := range []struct {
		siteID int
		hash   string
		when   time.Time
	}{
		{0, returning, now},
		{0, returning, firstDay.AddDate(0, 0, -1)}, // An earlier visit committed out of order.
		{0, first, firstDay},
		{0, first, now},
		{0, last, now},
		{2, last, firstDay.AddDate(0, 0, -1)}, // A different site's history.
		{2, last, firstDay},
		{0, first, now.AddDate(0, 0, 1)}, // Outside the report window.
	} {
		require.NoError(t, RecordSitePageView(ctx, visit.siteID, visit.hash, visit.when))
	}

	report, err := GetSiteTraffic(ctx, 0, 4, now)
	require.NoError(t, err)
	assert.Equal(t, SiteTrafficCounts{PageViews: 6, UniqueVisitors: 3, NewVisitors: 2}, report.Summary)
	assert.Equal(t, []SiteTrafficDay{
		{Date: "2026-11-18", SiteTrafficCounts: SiteTrafficCounts{PageViews: 3, UniqueVisitors: 2, NewVisitors: 1}},
		{Date: "2026-11-19"},
		{Date: "2026-11-20"},
		{Date: "2026-11-21", SiteTrafficCounts: SiteTrafficCounts{PageViews: 3, UniqueVisitors: 3, NewVisitors: 1}},
	}, report.Daily)
}
