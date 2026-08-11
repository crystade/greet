package http

import (
	nethttp "net/http"
	"net/url"
	"strings"
	"time"
)

// cookieEntry stores a parsed cookie with its computed expiry and scoping attributes.
type cookieEntry struct {
	name     string
	value    string
	domain   string    // normalized: lowercase, no leading dot
	hostOnly bool      // true when no Domain attribute was set (exact-host match only)
	path     string    // from Path attribute or defaulted from the request URL
	expires  time.Time // computed from Max-Age or Expires; zero means session (no expiry)
}

// cookieJar is a minimal in-memory cookie store for forwarding Set-Cookie
// headers across redirect hops within a single HTTP handshake. It is not a
// full RFC 6265 implementation — it only handles the subset needed for
// redirect chains (Domain, Path, Expires, Max-Age). Attributes like Secure,
// HttpOnly, SameSite, and Partitioned are ignored.
type cookieJar struct {
	entries []cookieEntry
	now     func() time.Time // injectable clock for testing
}

// newCookieJar returns a new empty cookie jar.
func newCookieJar() *cookieJar {
	return &cookieJar{
		now: time.Now,
	}
}

// setCookies processes Set-Cookie headers from a response and updates the jar.
// cookies should come from resp.Cookies() which parses the raw Set-Cookie
// headers into *http.Cookie structs with all attributes already populated.
//
// Handling per RFC 6265 §5.3:
//   - Max-Age < 0  → delete the matching cookie
//   - Max-Age > 0  → expires = now + Max-Age seconds
//   - Max-Age == 0 → attribute absent, fall back to Expires; if also absent → session cookie
//   - Domain: if empty, host-only cookie for the request host; if set, must domain-match the request host
//   - Path: if empty, defaults to the request URL's default-path (RFC 6265 §5.1.4)
//   - Duplicate name+domain+path → replace existing entry
func (j *cookieJar) setCookies(requestURL *url.URL, cookies []*nethttp.Cookie) {
	requestHost := strings.ToLower(requestURL.Hostname())

	for _, c := range cookies {
		if c.Name == "" {
			continue
		}

		// --- handle Max-Age deletion (Max-Age < 0 means delete) ---
		// In net/http: MaxAge < 0 means "delete", MaxAge == 0 means "not specified".
		if c.MaxAge < 0 {
			j.removeCookie(c.Name, requestHost, c.Path)
			continue
		}

		// --- compute expiry ---
		var expires time.Time
		if c.MaxAge > 0 {
			// Max-Age takes precedence over Expires (RFC 6265 §5.3 step 3).
			expires = j.now().Add(time.Duration(c.MaxAge) * time.Second)
		} else if !c.Expires.IsZero() {
			expires = c.Expires
		}
		// else: session cookie (expires stays zero)

		// If already expired, treat as deletion.
		if !expires.IsZero() && !j.now().Before(expires) {
			j.removeCookie(c.Name, requestHost, c.Path)
			continue
		}

		// --- resolve domain ---
		domain := strings.ToLower(c.Domain)
		domain = strings.TrimPrefix(domain, ".")
		hostOnly := false

		if domain == "" {
			// No Domain attribute → host-only: cookie is sent only to the exact host.
			domain = requestHost
			hostOnly = true
		} else {
			// Domain attribute present → validate that the request host domain-matches.
			if !domainMatches(requestHost, domain) {
				continue // reject: cookie domain doesn't match request host
			}
		}

		// --- resolve path ---
		path := c.Path
		if path == "" {
			path = defaultPath(requestURL.Path)
		}

		// --- upsert: replace existing entry with same name+domain+path ---
		replaced := false
		for i := range j.entries {
			if j.entries[i].name == c.Name && j.entries[i].domain == domain && j.entries[i].path == path {
				j.entries[i].value = c.Value
				j.entries[i].expires = expires
				j.entries[i].hostOnly = hostOnly
				replaced = true
				break
			}
		}
		if !replaced {
			j.entries = append(j.entries, cookieEntry{
				name:     c.Name,
				value:    c.Value,
				domain:   domain,
				hostOnly: hostOnly,
				path:     path,
				expires:  expires,
			})
		}
	}
}

// cookiesFor returns the Cookie header value to send for the given request URL.
// It filters entries by domain, path, and expiration. Returns an empty string
// when no cookies match.
func (j *cookieJar) cookiesFor(requestURL *url.URL) string {
	requestHost := strings.ToLower(requestURL.Hostname())
	requestPath := requestURL.Path
	if requestPath == "" {
		requestPath = "/"
	}

	var parts []string
	for _, e := range j.entries {
		// Check expiration.
		if !e.expires.IsZero() && !j.now().Before(e.expires) {
			continue
		}

		// Check domain.
		if e.hostOnly {
			if requestHost != e.domain {
				continue
			}
		} else {
			if !domainMatches(requestHost, e.domain) {
				continue
			}
		}

		// Check path.
		if !pathMatches(requestPath, e.path) {
			continue
		}

		parts = append(parts, e.name+"="+e.value)
	}

	return strings.Join(parts, "; ")
}

// removeCookie removes entries matching name and optionally domain/path.
func (j *cookieJar) removeCookie(name, host, path string) {
	host = strings.ToLower(host)
	n := 0
	for _, e := range j.entries {
		if e.name == name && e.domain == host && (path == "" || e.path == path) {
			continue // remove
		}
		j.entries[n] = e
		n++
	}
	j.entries = j.entries[:n]
}

// domainMatches reports whether the host domain-matches the cookie domain
// per RFC 6265 §5.1.3. The host matches if it equals the domain, or if
// the host is a subdomain of the domain.
func domainMatches(host, cookieDomain string) bool {
	if host == cookieDomain {
		return true
	}
	// host must end with "."+cookieDomain and the host must not be an IP.
	return strings.HasSuffix(host, "."+cookieDomain) && !isIPAddress(host)
}

// pathMatches reports whether the request path matches the cookie path
// per RFC 6265 §5.1.4.
func pathMatches(requestPath, cookiePath string) bool {
	if requestPath == cookiePath {
		return true
	}
	if strings.HasPrefix(requestPath, cookiePath) {
		// cookiePath is a prefix; either cookiePath ends with "/" or the next
		// character in requestPath after the prefix is "/".
		if strings.HasSuffix(cookiePath, "/") {
			return true
		}
		if len(requestPath) > len(cookiePath) && requestPath[len(cookiePath)] == '/' {
			return true
		}
	}
	return false
}

// defaultPath computes the "default-path" for a request URL per RFC 6265 §5.1.4.
func defaultPath(reqPath string) string {
	if reqPath == "" || reqPath[0] != '/' {
		return "/"
	}
	// Find the rightmost "/" (ignoring trailing content).
	i := strings.LastIndex(reqPath, "/")
	if i <= 0 {
		return "/"
	}
	return reqPath[:i]
}

// isIPAddress reports whether s looks like an IP address (v4 or v6).
func isIPAddress(s string) bool {
	// Strip brackets for IPv6 "[::1]".
	if len(s) > 0 && s[0] == '[' {
		s = strings.TrimRight(s[1:], "]")
	}
	for _, c := range s {
		if c != '.' && c != ':' && !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') && !(c >= 'A' && c <= 'F') {
			return false
		}
	}
	return len(s) > 0
}
