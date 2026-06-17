package router

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const sampleIndexHTML = `<!doctype html><html><head>` +
	`<title>New API</title>` +
	`<meta name="title" content="New API" />` +
	`</head><body><div id="root"></div></body></html>`

func newInjectTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c
}

// Main site (no resolved sub-site) falls back to the configured system name.
func TestRenderIndexHTML_UsesSystemName(t *testing.T) {
	old := common.SystemName
	common.SystemName = "Lemon Gateway"
	defer func() { common.SystemName = old }()

	out := string(renderIndexHTML(newInjectTestContext(), []byte(sampleIndexHTML)))
	if !strings.Contains(out, "<title>Lemon Gateway</title>") {
		t.Fatalf("title not injected with system name, got: %s", out)
	}
	if !strings.Contains(out, `<meta name="title" content="Lemon Gateway"`) {
		t.Fatalf("meta title not injected, got: %s", out)
	}
	if strings.Contains(out, "<title>New API</title>") {
		t.Fatalf("static title not replaced, got: %s", out)
	}
}

// A resolved white-label sub-site overrides the system name, and a name with
// HTML metacharacters must be escaped so it cannot break out of <title> or the
// content="…" attribute (XSS defense).
func TestRenderIndexHTML_SubSiteOverridesAndEscapes(t *testing.T) {
	c := newInjectTestContext()
	common.SetContextKey(c, constant.ContextKeySite, &model.Site{Name: `A&B "X" <script>`})

	out := string(renderIndexHTML(c, []byte(sampleIndexHTML)))
	if !strings.Contains(out, "<title>A&amp;B &#34;X&#34; &lt;script&gt;</title>") {
		t.Fatalf("escaped site title not injected, got: %s", out)
	}
	if strings.Contains(out, "<script>") {
		t.Fatalf("unescaped site name leaked into HTML (XSS), got: %s", out)
	}
}

// An empty resolvable name leaves the HTML untouched (no blank title).
func TestRenderIndexHTML_EmptyNameKeepsOriginal(t *testing.T) {
	old := common.SystemName
	common.SystemName = ""
	defer func() { common.SystemName = old }()

	out := renderIndexHTML(newInjectTestContext(), []byte(sampleIndexHTML))
	if string(out) != sampleIndexHTML {
		t.Fatalf("empty name should keep original HTML, got: %s", string(out))
	}
}
