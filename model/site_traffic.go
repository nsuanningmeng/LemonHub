package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SiteTrafficVisit holds one counter per browser and UTC day at a site.
// The secondary index supports finding that browser's visits on earlier days.
type SiteTrafficVisit struct {
	SiteID      int    `gorm:"primaryKey;autoIncrement:false;index:idx_site_traffic_history,priority:1"`
	VisitDate   string `gorm:"type:char(10);primaryKey;index:idx_site_traffic_history,priority:3"`
	VisitorHash string `gorm:"type:char(64);primaryKey;index:idx_site_traffic_history,priority:2"`
	PageViews   int64  `gorm:"not null"`
}

type SiteTrafficCounts struct {
	PageViews      int64 `json:"page_views"`
	UniqueVisitors int64 `json:"unique_visitors"`
	NewVisitors    int64 `json:"new_visitors"`
}

type SiteTrafficDay struct {
	Date string `json:"date" gorm:"column:visit_date"`
	SiteTrafficCounts
}

type SiteTrafficReport struct {
	Summary SiteTrafficCounts `json:"summary"`
	Daily   []SiteTrafficDay  `json:"daily"`
}

const MaxSiteTrafficDays = 90

func RecordSitePageView(ctx context.Context, siteID int, visitorHash string, at time.Time) error {
	if siteID < 0 {
		return errors.New("invalid traffic site")
	}
	if len(visitorHash) != sha256.Size*2 {
		return errors.New("invalid traffic visitor hash")
	}
	if _, err := hex.DecodeString(visitorHash); err != nil {
		return errors.New("invalid traffic visitor hash")
	}

	visit := SiteTrafficVisit{
		SiteID:      siteID,
		VisitDate:   at.UTC().Format(time.DateOnly),
		VisitorHash: strings.ToLower(visitorHash),
		PageViews:   1,
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "site_id"}, {Name: "visit_date"}, {Name: "visitor_hash"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"page_views": gorm.Expr("site_traffic_visits.page_views + 1"),
		}),
	}).Create(&visit).Error
}

func GetSiteTraffic(ctx context.Context, siteID int, days int, now time.Time) (*SiteTrafficReport, error) {
	if siteID < 0 || days < 1 || days > MaxSiteTrafficDays {
		return nil, errors.New("invalid traffic report range or site")
	}
	today := now.UTC()
	start := today.AddDate(0, 0, 1-days)
	firstDate := start.Format(time.DateOnly)

	// Each browser contributes one row to its day's UV. Count it towards the
	// period's UV on its first day in the period, and towards new visitors only
	// on its first recorded day at this site. All counts share one DB snapshot.
	const earlierVisits = `SELECT 1 FROM site_traffic_visits AS earlier
		WHERE earlier.site_id = visits.site_id AND earlier.visitor_hash = visits.visitor_hash
		AND earlier.visit_date < visits.visit_date`
	var counts []struct {
		SiteTrafficDay
		FirstInPeriod int64
	}
	err := DB.WithContext(ctx).Table("site_traffic_visits AS visits").
		Select(`visits.visit_date, SUM(visits.page_views) AS page_views, COUNT(*) AS unique_visitors,
			SUM(CASE WHEN NOT EXISTS (`+earlierVisits+`) THEN 1 ELSE 0 END) AS new_visitors,
			SUM(CASE WHEN NOT EXISTS (`+earlierVisits+` AND earlier.visit_date >= ?) THEN 1 ELSE 0 END) AS first_in_period`, firstDate).
		Where("visits.site_id = ? AND visits.visit_date BETWEEN ? AND ?", siteID, firstDate, today.Format(time.DateOnly)).
		Group("visits.visit_date").Order("visits.visit_date").Scan(&counts).Error
	if err != nil {
		return nil, err
	}

	report := &SiteTrafficReport{Daily: make([]SiteTrafficDay, days)}
	next := 0
	for i := range report.Daily {
		date := start.AddDate(0, 0, i).Format(time.DateOnly)
		report.Daily[i].Date = date
		if next < len(counts) && counts[next].Date == date {
			row := counts[next]
			report.Daily[i] = row.SiteTrafficDay
			report.Summary.PageViews += row.PageViews
			report.Summary.UniqueVisitors += row.FirstInPeriod
			report.Summary.NewVisitors += row.NewVisitors
			next++
		}
	}
	return report, nil
}
