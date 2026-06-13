package model

import (
	"errors"
	"net"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// Site represents a white-label sub-site (tenant). Id 0 is reserved for the main
// site and is never persisted as an actual row.
//
// Amounts (WalletBalance, WalletWarnThreshold) are stored as integers in 厘
// (0.001 CNY) — never floats. DiscountRate is a basis-of-10000 integer where
// 10000 means no discount and 7000 means 70% (七折). Cost = face * DiscountRate / 10000.
type Site struct {
	Id                  int    `json:"id" gorm:"primaryKey"`
	Name                string `json:"name" gorm:"type:varchar(128)"`
	Logo                string `json:"logo" gorm:"type:varchar(512)"`
	Notice              string `json:"notice" gorm:"type:text"`
	Footer              string `json:"footer" gorm:"type:text"`
	OwnerUserId         int    `json:"owner_user_id" gorm:"index"`
	Status              int    `json:"status" gorm:"type:int;default:1"`            // 1 normal, 2 disabled
	WalletBalance       int64  `json:"wallet_balance" gorm:"type:bigint;default:0"` // 厘
	DiscountRate        int    `json:"discount_rate" gorm:"type:int;default:10000"` // 万分比, 10000 = 原价
	WalletWarnThreshold int64  `json:"wallet_warn_threshold" gorm:"type:bigint;default:0"`
	PayConfig           string `json:"pay_config" gorm:"type:text"` // JSON, per-site payment config (used from phase 4)
	CreatedTime         int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime         int64  `json:"updated_time" gorm:"bigint"`

	// Transient fields, never persisted by GORM.
	Domains       []string `json:"domains" gorm:"-"`
	OwnerUsername string   `json:"owner_username" gorm:"-"`
}

// SiteDomain binds a domain to a sub-site. A domain belongs to at most one site
// (enforced by the unique index).
type SiteDomain struct {
	Id     int    `json:"id" gorm:"primaryKey"`
	SiteId int    `json:"site_id" gorm:"index"`
	Domain string `json:"domain" gorm:"type:varchar(255);uniqueIndex"`
}

const (
	SiteStatusNormal   = 1
	SiteStatusDisabled = 2

	// DiscountRateBase is the denominator for DiscountRate (万分比).
	DiscountRateBase = 10000

	// SiteScopeAll is the sentinel passed to site-aware admin query helpers to mean
	// "no site filter" (main-site admins see every sub-site). A value >= 0 restricts
	// the query to that specific site_id (0 = main site).
	SiteScopeAll = -1
	// SiteScopeDenied is the fail-closed sentinel for a scoped operator whose own site
	// could not be determined. It is filtered as `site_id = -2`, which matches no rows,
	// so a request can never widen to the main site by accident.
	SiteScopeDenied = -2
)

// ---- In-memory cache: domain -> *Site and id -> *Site ----

var (
	siteCacheMutex  sync.RWMutex
	siteDomainCache = map[string]*Site{}
	siteIdCache     = map[int]*Site{}
)

// NormalizeDomain reduces a Host header (or an admin-entered domain) to a bare,
// comparable hostname: lowercased, scheme/userinfo/path/port stripped, IPv6
// brackets and a trailing FQDN dot removed. This keeps admin-configured domains
// and incoming Host headers matching consistently.
func NormalizeDomain(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return ""
	}
	// Strip scheme if an admin pasted a full URL (e.g. https://example.com/).
	if i := strings.Index(host, "://"); i != -1 {
		host = host[i+3:]
	}
	// Strip any path/query/fragment.
	if i := strings.IndexAny(host, "/?#"); i != -1 {
		host = host[:i]
	}
	// Strip userinfo (user:pass@host).
	if i := strings.LastIndex(host, "@"); i != -1 {
		host = host[i+1:]
	}
	// Strip port (also unwraps bracketed IPv6 with a port, e.g. [::1]:443 -> ::1).
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	// Unwrap bracketed IPv6 that had no port, then drop a trailing FQDN dot.
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	host = strings.TrimSuffix(host, ".")
	return host
}

// ReloadSiteCache rebuilds the domain/id caches from the database. Call on startup
// and after any site/domain mutation. It swaps the maps atomically.
func ReloadSiteCache() error {
	var sites []*Site
	if err := DB.Find(&sites).Error; err != nil {
		return err
	}
	var domains []SiteDomain
	if err := DB.Find(&domains).Error; err != nil {
		return err
	}

	idCache := make(map[int]*Site, len(sites))
	for _, s := range sites {
		idCache[s.Id] = s
	}
	domainCache := make(map[string]*Site, len(domains))
	for _, d := range domains {
		if s, ok := idCache[d.SiteId]; ok {
			s.Domains = append(s.Domains, d.Domain)
			domainCache[NormalizeDomain(d.Domain)] = s
		}
	}

	siteCacheMutex.Lock()
	siteDomainCache = domainCache
	siteIdCache = idCache
	siteCacheMutex.Unlock()
	return nil
}

// GetSiteByDomainCached returns the sub-site bound to a host, or nil for the main site.
func GetSiteByDomainCached(host string) *Site {
	domain := NormalizeDomain(host)
	if domain == "" {
		return nil
	}
	siteCacheMutex.RLock()
	defer siteCacheMutex.RUnlock()
	return siteDomainCache[domain]
}

// GetSiteByIdCached returns a cached *Site by id, or nil if not found / main site.
func GetSiteByIdCached(id int) *Site {
	if id == 0 {
		return nil
	}
	siteCacheMutex.RLock()
	defer siteCacheMutex.RUnlock()
	return siteIdCache[id]
}

// reloadSiteCacheSoft refreshes the domain cache after a committed mutation. The
// DB write already succeeded and is durable, so a transient reload failure is
// logged (the cache self-heals on the next mutation or restart) rather than
// surfaced as a misleading operation error to the admin.
func reloadSiteCacheSoft() {
	if err := ReloadSiteCache(); err != nil {
		common.SysError("failed to reload sub-site cache after mutation: " + err.Error())
	}
}

// ---- Persistence / CRUD ----

// fillSiteAux fills transient fields (Domains, OwnerUsername) for the given sites.
// It returns an error so callers never present an admin a "successful" response
// built on a silently-failed read.
func fillSiteAux(sites []*Site) error {
	if len(sites) == 0 {
		return nil
	}
	siteIds := make([]int, 0, len(sites))
	ownerIds := make([]int, 0, len(sites))
	byId := make(map[int]*Site, len(sites))
	for _, s := range sites {
		s.Domains = []string{}
		siteIds = append(siteIds, s.Id)
		ownerIds = append(ownerIds, s.OwnerUserId)
		byId[s.Id] = s
	}

	var domains []SiteDomain
	if err := DB.Where("site_id IN ?", siteIds).Find(&domains).Error; err != nil {
		return err
	}
	for _, d := range domains {
		if s, ok := byId[d.SiteId]; ok {
			s.Domains = append(s.Domains, d.Domain)
		}
	}

	type ownerRow struct {
		Id       int
		Username string
	}
	var owners []ownerRow
	if err := DB.Model(&User{}).Select("id, username").Where("id IN ?", ownerIds).Find(&owners).Error; err != nil {
		return err
	}
	ownerMap := make(map[int]string, len(owners))
	for _, o := range owners {
		ownerMap[o.Id] = o.Username
	}
	for _, s := range sites {
		s.OwnerUsername = ownerMap[s.OwnerUserId]
	}
	return nil
}

// GetAllSites returns a page of sites ordered by id desc, with total count.
func GetAllSites(startIdx int, num int) ([]*Site, int64, error) {
	var sites []*Site
	var total int64
	if err := DB.Model(&Site{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := DB.Order("id desc").Limit(num).Offset(startIdx).Find(&sites).Error
	if err != nil {
		return nil, 0, err
	}
	if err := fillSiteAux(sites); err != nil {
		return nil, 0, err
	}
	return sites, total, nil
}

// SearchSites searches sites by name or domain.
func SearchSites(keyword string, startIdx int, num int) ([]*Site, int64, error) {
	var sites []*Site
	var total int64
	keyword = strings.TrimSpace(keyword)
	// Escape LIKE wildcards/specials in the user keyword (consistent with the rest of the
	// repo), then wrap as a substring pattern with an explicit ESCAPE char.
	escaped, err := sanitizeLikePattern(keyword)
	if err != nil {
		return nil, 0, err
	}
	like := "%" + escaped + "%"

	// Match by site name, or by any bound domain.
	sub := DB.Model(&SiteDomain{}).Select("site_id").Where("domain LIKE ? ESCAPE '!'", like)
	query := DB.Model(&Site{}).Where("name LIKE ? ESCAPE '!' OR id IN (?)", like, sub)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id desc").Limit(num).Offset(startIdx).Find(&sites).Error; err != nil {
		return nil, 0, err
	}
	if err := fillSiteAux(sites); err != nil {
		return nil, 0, err
	}
	return sites, total, nil
}

// GetSiteById returns a single site with transient fields populated.
func GetSiteById(id int) (*Site, error) {
	if id == 0 {
		return nil, errors.New("invalid site id")
	}
	var site Site
	if err := DB.First(&site, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if err := fillSiteAux([]*Site{&site}); err != nil {
		return nil, err
	}
	return &site, nil
}

// normalizeDomains cleans a domain list: lowercases, strips ports, trims, dedupes
// while preserving order, and drops blanks.
func normalizeDomains(domains []string) []string {
	seen := make(map[string]struct{}, len(domains))
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		n := NormalizeDomain(d)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

// checkDomainConflict ensures none of the domains are already bound to a different site.
// excludeSiteId is the site being updated (0 when creating).
func checkDomainConflict(tx *gorm.DB, domains []string, excludeSiteId int) error {
	if len(domains) == 0 {
		return nil
	}
	var existing []SiteDomain
	if err := tx.Where("domain IN ?", domains).Find(&existing).Error; err != nil {
		return err
	}
	for _, e := range existing {
		if e.SiteId != excludeSiteId {
			return errors.New("域名已被其他子站绑定: " + e.Domain)
		}
	}
	return nil
}

// resolveOwner looks up a sub-site owner by username among MAIN-SITE accounts
// (site_id = 0). Since usernames are only unique per-site now, a global lookup would be
// ambiguous; a sub-site owner is always a main-site agent account. Platform admins/root
// are rejected — their main-site privileges must not be scoped down to a sub-site.
func resolveOwner(tx *gorm.DB, username string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("归属代理账号不能为空")
	}
	var user User
	if err := tx.Where("username = ? AND site_id = ?", username, 0).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("归属代理账号不存在（须为主站账号）: " + username)
		}
		return nil, err
	}
	if user.Role >= common.RoleAdminUser {
		return nil, errors.New("归属账号不能是平台管理员")
	}
	return &user, nil
}

// CreateSite creates a sub-site and its domains atomically, then reloads the cache.
// site.Domains and site.OwnerUsername drive the domain bindings and owner resolution.
func CreateSite(site *Site) error {
	domains := normalizeDomains(site.Domains)
	if site.Name == "" {
		return errors.New("子站名称不能为空")
	}
	if len(domains) == 0 {
		return errors.New("至少需要绑定一个域名")
	}
	if site.Status != SiteStatusNormal && site.Status != SiteStatusDisabled {
		site.Status = SiteStatusNormal
	}
	if site.DiscountRate <= 0 {
		site.DiscountRate = DiscountRateBase
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		owner, err := resolveOwner(tx, site.OwnerUsername)
		if err != nil {
			return err
		}
		if err := checkDomainConflict(tx, domains, 0); err != nil {
			return err
		}
		site.OwnerUserId = owner.Id
		site.CreatedTime = common.GetTimestamp()
		site.UpdatedTime = site.CreatedTime
		// Persist core row (transient Domains/OwnerUsername are gorm:"-").
		if err := tx.Create(site).Error; err != nil {
			return err
		}
		for _, d := range domains {
			if err := tx.Create(&SiteDomain{SiteId: site.Id, Domain: d}).Error; err != nil {
				return err
			}
		}
		// Promote the owner to a sub-site admin and bind their account to this sub-site,
		// so they administer it via SiteAdminAuth-gated endpoints. Role is only raised
		// (never lowered) for a common user; site_id is repointed to the new sub-site
		// (the brand-new site has no users, so the composite (site_id,username) is free).
		ownerUpdates := map[string]interface{}{"site_id": site.Id}
		if owner.Role < common.RoleSubSiteAdmin {
			ownerUpdates["role"] = common.RoleSubSiteAdmin
		}
		if err := tx.Model(&User{}).Where("id = ?", owner.Id).Updates(ownerUpdates).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	site.Domains = domains
	reloadSiteCacheSoft()
	return nil
}

// UpdateSite updates a site's mutable fields and (optionally) re-binds its domains.
func UpdateSite(site *Site) error {
	if site.Id == 0 {
		return errors.New("invalid site id")
	}
	domains := normalizeDomains(site.Domains)
	if len(domains) == 0 {
		return errors.New("至少需要绑定一个域名")
	}
	if site.Status != SiteStatusNormal && site.Status != SiteStatusDisabled {
		site.Status = SiteStatusNormal
	}
	if site.DiscountRate <= 0 {
		site.DiscountRate = DiscountRateBase
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing Site
		if err := tx.First(&existing, "id = ?", site.Id).Error; err != nil {
			return err
		}
		// The owner is fixed at creation (it carries a role+site_id promotion that is not
		// safe to flip in an update); branding/status/discount edits keep the same owner.
		if err := checkDomainConflict(tx, domains, site.Id); err != nil {
			return err
		}

		updates := map[string]interface{}{
			"name":                  site.Name,
			"logo":                  site.Logo,
			"notice":                site.Notice,
			"footer":                site.Footer,
			"status":                site.Status,
			"discount_rate":         site.DiscountRate,
			"wallet_warn_threshold": site.WalletWarnThreshold,
			"pay_config":            site.PayConfig,
			"updated_time":          common.GetTimestamp(),
		}
		if err := tx.Model(&Site{}).Where("id = ?", site.Id).Updates(updates).Error; err != nil {
			return err
		}
		// Re-bind domains: delete then recreate (small set, simplest correct approach).
		if err := tx.Where("site_id = ?", site.Id).Delete(&SiteDomain{}).Error; err != nil {
			return err
		}
		for _, d := range domains {
			if err := tx.Create(&SiteDomain{SiteId: site.Id, Domain: d}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	reloadSiteCacheSoft()
	return nil
}

// UpdateSiteBranding updates ONLY a sub-site's four brand fields (name/logo/notice/footer)
// plus updated_time, then refreshes the cache. A sub-site admin may edit branding but never
// domains/discount_rate/owner/status/pay_config, so this helper deliberately touches no other
// columns. A column map (not a struct) is used so that clearing notice/footer to empty strings
// is actually persisted — GORM struct Updates would skip those zero values.
func UpdateSiteBranding(siteId int, name, logo, notice, footer string) error {
	if siteId <= 0 {
		return errors.New("无效的子站")
	}
	if name == "" {
		return errors.New("子站名称不能为空")
	}
	updates := map[string]interface{}{
		"name":         name,
		"logo":         logo,
		"notice":       notice,
		"footer":       footer,
		"updated_time": common.GetTimestamp(),
	}
	res := DB.Model(&Site{}).Where("id = ?", siteId).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("子站不存在")
	}
	reloadSiteCacheSoft()
	return nil
}

// DeleteSite removes a site and its domain bindings, then reloads the cache.
func DeleteSite(id int) error {
	if id == 0 {
		return errors.New("invalid site id")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Lock/delete the sites row FIRST (same Site-then-Domain order as UpdateSite) so
		// concurrent Update/Delete on the same site id cannot ABBA-deadlock on MySQL/PG.
		res := tx.Delete(&Site{}, "id = ?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("子站不存在")
		}
		if err := tx.Where("site_id = ?", id).Delete(&SiteDomain{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	reloadSiteCacheSoft()
	return nil
}
