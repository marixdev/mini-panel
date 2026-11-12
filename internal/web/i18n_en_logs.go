package web

// English text of the Logs tab of a website.
func init() {
	for k, v := range map[string]string{
		"tab.logs":        "Logs",
		"log.title":       "Website logs",
		"log.sub":         "Latest log lines, newest at the bottom. Read only.",
		"log.kinds":       "Log type",
		"log.kind.access": "Access",
		"log.kind.error":  "Error",
		"log.kind.app":    "Application",
		"log.search":      "Show only lines containing",
		"log.search.ph":   "e.g. 404, an IP address or a path",
		"log.lines":       "Lines",
		"log.show":        "Show",
		"log.size":        "Full file size",
		"log.matched":     "Matching lines",
		"log.truncated":   "Large file: searching only the end",
		"log.view":        "Log contents",
		"log.missing":     "No log at %s yet. It is created when the website receives its first request.",
		"log.empty":       "The log is empty.",
		"log.nomatch":     "No matching lines in the end of this log.",
		"log.hint":        "Access logs record requests; error logs record nginx errors. Node and Python application logs come from the website service. Logs rotate daily and 14 copies are kept.",
		"log.err.query":   "Invalid search text; the filter was not applied.",
	} {
		en[k] = v
	}
}
