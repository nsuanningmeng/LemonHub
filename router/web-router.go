package router

import (
	"embed"
	"html"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// ThemeAssets holds the embedded frontend assets for both themes.
type ThemeAssets struct {
	DefaultBuildFS   embed.FS
	DefaultIndexPage []byte
	ClassicBuildFS   embed.FS
	ClassicIndexPage []byte
}

// indexTitleRegexp matches the static <title>…</title> in the embedded index
// HTML; indexMetaTitleRegexp matches the <meta name="title" content="…"> tag.
// Both are rewritten per request so the browser shows the correct site name on
// first paint (see renderIndexHTML).
var (
	indexTitleRegexp = regexp.MustCompile(`(?is)<title>.*?</title>`)
	// Matches the WHOLE <meta name="title" …> tag, including its closing `>`/`/>`,
	// so the replacement emits a complete, well-formed tag (never a dangling
	// `content="…"` fragment) regardless of how the build output self-closes it.
	indexMetaTitleRegexp = regexp.MustCompile(`(?is)<meta\s+name="title"\s+content="[^"]*"\s*/?>`)
)

func SetWebRouter(router *gin.Engine, assets ThemeAssets) {
	defaultFS := common.EmbedFolder(assets.DefaultBuildFS, "web/default/dist")
	classicFS := common.EmbedFolder(assets.ClassicBuildFS, "web/classic/dist")
	themeFS := common.NewThemeAwareFS(defaultFS, classicFS)

	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimit())
	router.Use(middleware.Cache())
	router.Use(static.Serve("/", themeFS))
	router.NoRoute(func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}
		c.Header("Cache-Control", "no-cache")
		// The injected title is per-Host, so a shared cache must key on Host —
		// otherwise domain A's title HTML could be served to domain B.
		c.Header("Vary", "Host")
		// The embedded FS routes "/" (and every SPA path) here, so this is the
		// single place every HTML entry point is served — inject the per-domain
		// site name into the title before sending.
		if common.GetTheme() == "classic" {
			c.Data(http.StatusOK, "text/html; charset=utf-8", renderIndexHTML(c, assets.ClassicIndexPage))
		} else {
			c.Data(http.StatusOK, "text/html; charset=utf-8", renderIndexHTML(c, assets.DefaultIndexPage))
		}
	})
}

// renderIndexHTML injects the resolved site name into the static index HTML's
// <title> (and matching <meta name="title">) so the browser tab shows the correct
// site name on first paint instead of flashing the build-time default ("New API")
// until client JS fetches /api/status. For a white-label sub-site the site's own
// name is used; otherwise the configured system name. The response is served
// no-cache (see the NoRoute handler), so per-domain titles are never cached across
// domains. Returns the bytes unchanged when no name is available.
func renderIndexHTML(c *gin.Context, index []byte) []byte {
	name := strings.TrimSpace(resolveSiteName(c))
	if name == "" {
		return index
	}
	escaped := html.EscapeString(name)
	titleTag := []byte("<title>" + escaped + "</title>")
	metaTag := []byte(`<meta name="title" content="` + escaped + `" />`)
	// ReplaceAllFunc (vs ReplaceAll) avoids $-template expansion, so a site name
	// containing '$' is injected verbatim.
	out := indexTitleRegexp.ReplaceAllFunc(index, func([]byte) []byte { return titleTag })
	out = indexMetaTitleRegexp.ReplaceAllFunc(out, func([]byte) []byte { return metaTag })
	return out
}

// resolveSiteName returns the white-label sub-site's name for the current request
// (resolved by middleware.SiteResolver from the Host header), falling back to the
// global system name.
func resolveSiteName(c *gin.Context) string {
	if site := middleware.GetRequestSite(c); site != nil {
		if name := strings.TrimSpace(site.Name); name != "" {
			return name
		}
	}
	return common.SystemName
}
