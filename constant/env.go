package constant

import "sync/atomic"

var StreamingTimeout int
var DifyDebug bool
var MaxFileDownloadMB int
var StreamScannerMaxBufferMB int
var ForceStreamOption bool
var CountToken bool
var GetMediaToken bool
var GetMediaTokenNotStream bool
var UpdateTask bool
var MaxRequestBodyMB int
var AnonymousRequestBodyLimitKB int
var AzureDefaultAPIVersion string
var NotifyLimitCount int
var NotificationLimitDurationMinute int
var GenerateDefaultToken bool
var ErrorLogEnabled bool
var TaskQueryLimit int
var TaskTimeoutMinutes int

// temporary variable for sora patch, will be removed in future
var TaskPricePatches []string

// Trusted redirect domains are read on the request hot path (redirect URL
// validation, payment base-URL derivation) while the option-provided list is
// re-published at runtime whenever an admin saves options or the periodic
// option sync runs. Storing each list behind an atomic pointer makes the
// writer's whole-slice swap and the readers' loads race-free: readers always
// observe a fully-populated slice, never a torn header. The published slices
// are treated as immutable after Set — callers must not mutate them in place.
var (
	trustedRedirectDomainsEnv    atomic.Pointer[[]string] // from TRUSTED_REDIRECT_DOMAINS (set once at init)
	trustedRedirectDomainsOption atomic.Pointer[[]string] // from the TrustedRedirectDomains option (runtime-editable)
)

// SetTrustedRedirectDomains publishes the env-provided trusted redirect domain
// list. Domains support subdomain matching (e.g. "example.com" matches
// "sub.example.com").
func SetTrustedRedirectDomains(domains []string) {
	trustedRedirectDomainsEnv.Store(&domains)
}

// SetTrustedRedirectDomainsFromOption publishes the admin-configured trusted
// redirect domain list (option key "TrustedRedirectDomains"). It is unioned
// with the env-provided list.
func SetTrustedRedirectDomainsFromOption(domains []string) {
	trustedRedirectDomainsOption.Store(&domains)
}

// TrustedRedirectDomainLists returns the env and option trusted-domain lists as
// a slice of read-only lists (nil pointers become empty). Callers iterate; they
// must not mutate the returned slices.
func TrustedRedirectDomainLists() [][]string {
	lists := make([][]string, 0, 2)
	if p := trustedRedirectDomainsEnv.Load(); p != nil {
		lists = append(lists, *p)
	}
	if p := trustedRedirectDomainsOption.Load(); p != nil {
		lists = append(lists, *p)
	}
	return lists
}
