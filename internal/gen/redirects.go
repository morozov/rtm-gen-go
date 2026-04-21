package gen

import "strings"

// rtmDocsBase is the origin RTM uses for relative-path anchors
// in its reflection spec.
const rtmDocsBase = "https://www.rememberthemilk.com"

// knownRedirects maps the relative paths RTM emits in <a href>
// values to their final resolved HTTPS URLs. RTM keeps several
// pre-redirect legacy paths in the reflection spec; leaving them
// raw means every link a user clicks takes a permanent-redirect
// round-trip. Resolving once at generator build time keeps the
// committed output clean.
//
// Paths not in this map pass through resolveDocsURL() as
// rtmDocsBase + path with no redirect resolution. When RTM adds
// a new legacy URL we don't yet know about, the CLI still emits
// a working link.
var knownRedirects = map[string]string{
	"/services/api/keys.rtm":                          "https://www.rememberthemilk.com/services/api/keys.rtm",
	"/services/api/timelines.rtm":                     "https://www.rememberthemilk.com/services/api/timelines.rtm",
	"/services/api/methods/rtm.time.parse.rtm":        "https://www.rememberthemilk.com/services/api/methods/rtm.time.parse.rtm",
	"/services/api/methods/rtm.timezones.getList.rtm": "https://www.rememberthemilk.com/services/api/methods/rtm.timezones.getList.rtm",
	"/help/answers/search/advanced.rtm":               "https://www.rememberthemilk.com/help/?ctx=basics.search.advanced",
	"/help/answers/basics/repeatformat.rtm":           "https://www.rememberthemilk.com/help/?ctx=basics.basics.repeatformat",
	"/services/smartadd":                              "https://www.rememberthemilk.com/help/?ctx=basics.smartadd.whatis",
}

// resolveDocsURL turns an anchor href from an RTM description
// into the URL we actually emit in CLI footnotes and manifests.
// Known legacy paths are replaced with their resolved final URL;
// unknown relative paths are prefixed with the RTM origin;
// absolute URLs pass through unchanged.
func resolveDocsURL(href string) string {
	if resolved, ok := knownRedirects[href]; ok {
		return resolved
	}
	if strings.HasPrefix(href, "/") {
		return rtmDocsBase + href
	}
	return href
}
