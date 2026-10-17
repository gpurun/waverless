package gongji

import (
	"net/url"
	"strconv"
	"strings"
)

// PointBackendURL returns an HTTPS origin for Traefik upstream (Gongji 550w.link style).
func PointBackendURL(pt Point, defaultPort int) string {
	for _, raw := range []string{
		strings.TrimSpace(pt.AccessURL),
		strings.TrimSpace(pt.URL),
		strings.TrimSpace(pt.ServiceURL),
		strings.TrimSpace(pt.RemoteURL),
		strings.TrimSpace(pt.Domain),
	} {
		if u := normalizeBackendOrigin(raw, defaultPort); u != "" {
			return u
		}
	}
	name := strings.TrimSpace(pt.Name)
	if strings.Contains(name, ".") && (strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://")) {
		return normalizeBackendOrigin(name, defaultPort)
	}
	if strings.Contains(name, "550w.link") || strings.Contains(name, ".link") {
		return normalizeBackendOrigin("https://"+strings.TrimPrefix(name, "https://"), defaultPort)
	}
	return ""
}

func normalizeBackendOrigin(raw string, defaultPort int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	// Public Gongji URLs are full https origins (550w.link); do not append worker port.
	if !strings.Contains(u.Host, ":") && defaultPort > 0 {
		if u.Scheme == "http" && defaultPort != 80 {
			u.Host = u.Host + ":" + strconv.Itoa(defaultPort)
		} else if u.Scheme == "https" && defaultPort != 443 && defaultPort != 80 {
			// Only append for non-standard https ports (internal dev).
			if !strings.HasSuffix(strings.ToLower(u.Host), ".550w.link") &&
				!strings.HasSuffix(strings.ToLower(u.Host), ".link") {
				u.Host = u.Host + ":" + strconv.Itoa(defaultPort)
			}
		}
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
