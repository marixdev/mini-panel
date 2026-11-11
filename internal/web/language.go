package web

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// A translator belongs to one request. No global language is mutated.
type translator func(string, ...any) string

func translate(language, key string, args ...any) string {
	if language == "en" {
		if text, ok := en[key]; ok {
			return formatText(text, args...)
		}
	}
	return T(key, args...)
}

func translatorFor(language string) translator {
	return func(key string, args ...any) string { return translate(language, key, args...) }
}

func requestLanguage(r *http.Request) string {
	if c, err := r.Cookie("mpn_language"); err == nil && (c.Value == "en" || c.Value == "vi") {
		return c.Value
	}
	// Vietnamese remains the default. Match the browser's supported preference,
	// honoring quality weights and ignoring explicitly excluded languages.
	best, quality := "vi", 0.0
	for _, preference := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		parts := strings.Split(strings.TrimSpace(preference), ";")
		language := strings.ToLower(strings.Split(parts[0], "-")[0])
		if language != "en" && language != "vi" {
			continue
		}
		q := 1.0
		for _, parameter := range parts[1:] {
			parameter = strings.TrimSpace(parameter)
			if strings.HasPrefix(parameter, "q=") {
				value, err := strconv.ParseFloat(strings.TrimPrefix(parameter, "q="), 64)
				if err != nil || value < 0 || value > 1 {
					q = 0
				} else {
					q = value
				}
			}
		}
		if q > quality {
			best, quality = language, q
		}
	}
	return best
}

func translatorForRequest(r *http.Request) translator { return translatorFor(requestLanguage(r)) }

func writerLanguage(w io.Writer) string {
	if h, ok := w.(interface{ Header() http.Header }); ok && h.Header().Get("Content-Language") == "en" {
		return "en"
	}
	return "vi"
}

func translatorForWriter(w io.Writer) translator { return translatorFor(writerLanguage(w)) }

// This preference does not require sign-in, so setup and login can be localized.
func (s *Server) changeLanguage(w http.ResponseWriter, r *http.Request) {
	language := r.PathValue("language")
	if language != "en" && language != "vi" {
		http.NotFound(w, r)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "mpn_language", Value: language, Path: "/", MaxAge: 365 * 24 * 60 * 60, Expires: time.Now().AddDate(1, 0, 0), HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteLaxMode})
	back := "/"
	if u, err := url.Parse(r.Referer()); err == nil && strings.EqualFold(u.Host, r.Host) && (u.Scheme == "http" || u.Scheme == "https") {
		if strings.HasPrefix(u.Path, "/") && !strings.HasPrefix(u.Path, "//") && !strings.HasPrefix(u.Path, "/language/") && !strings.ContainsAny(u.Path, "\r\n\\") {
			back = u.RequestURI()
		}
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}
