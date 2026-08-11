package http

import (
	nethttp "net/http"
	"net/url"
	"testing"
	"time"
)

// fixedNow returns a function that always returns the given time.
func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("bad URL %q: %v", raw, err)
	}
	return u
}

func TestCookieJar_BasicForwarding(t *testing.T) {
	jar := newCookieJar()
	reqURL := mustParseURL(t, "https://example.com/login")

	jar.setCookies(reqURL, []*nethttp.Cookie{
		{Name: "sid", Value: "abc123"},
	})

	got := jar.cookiesFor(reqURL)
	if got != "sid=abc123" {
		t.Errorf("cookiesFor = %q, want %q", got, "sid=abc123")
	}
}

func TestCookieJar_MultipleCookies(t *testing.T) {
	jar := newCookieJar()
	reqURL := mustParseURL(t, "https://example.com/")

	jar.setCookies(reqURL, []*nethttp.Cookie{
		{Name: "a", Value: "1"},
		{Name: "b", Value: "2"},
	})

	got := jar.cookiesFor(reqURL)
	if got != "a=1; b=2" {
		t.Errorf("cookiesFor = %q, want %q", got, "a=1; b=2")
	}
}

func TestCookieJar_DomainScoping(t *testing.T) {
	jar := newCookieJar()
	setURL := mustParseURL(t, "https://auth.example.com/login")

	// Cookie with Domain=example.com should be sent to subdomains.
	jar.setCookies(setURL, []*nethttp.Cookie{
		{Name: "token", Value: "xyz", Domain: "example.com"},
	})

	tests := []struct {
		name string
		url  string
		want string
	}{
		{"exact domain", "https://example.com/", "token=xyz"},
		{"subdomain match", "https://auth.example.com/", "token=xyz"},
		{"other subdomain", "https://api.example.com/", "token=xyz"},
		{"unrelated domain", "https://other.com/", ""},
		{"partial suffix", "https://notexample.com/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jar.cookiesFor(mustParseURL(t, tt.url))
			if got != tt.want {
				t.Errorf("cookiesFor(%s) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestCookieJar_HostOnlyCookie(t *testing.T) {
	jar := newCookieJar()
	setURL := mustParseURL(t, "https://example.com/")

	// No Domain attribute → host-only cookie.
	jar.setCookies(setURL, []*nethttp.Cookie{
		{Name: "sess", Value: "abc"},
	})

	tests := []struct {
		name string
		url  string
		want string
	}{
		{"exact host", "https://example.com/", "sess=abc"},
		{"subdomain rejected", "https://sub.example.com/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jar.cookiesFor(mustParseURL(t, tt.url))
			if got != tt.want {
				t.Errorf("cookiesFor(%s) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestCookieJar_PathScoping(t *testing.T) {
	jar := newCookieJar()
	setURL := mustParseURL(t, "https://example.com/app/login")

	jar.setCookies(setURL, []*nethttp.Cookie{
		{Name: "sid", Value: "123", Path: "/app"},
	})

	tests := []struct {
		name string
		url  string
		want string
	}{
		{"exact path", "https://example.com/app", "sid=123"},
		{"sub path", "https://example.com/app/dashboard", "sid=123"},
		{"root rejected", "https://example.com/", ""},
		{"other path rejected", "https://example.com/api", ""},
		{"partial prefix rejected", "https://example.com/application", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jar.cookiesFor(mustParseURL(t, tt.url))
			if got != tt.want {
				t.Errorf("cookiesFor(%s) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestCookieJar_DefaultPath(t *testing.T) {
	jar := newCookieJar()
	setURL := mustParseURL(t, "https://example.com/a/b/c")

	// No Path attribute → default-path is "/a/b" (up to the last "/").
	jar.setCookies(setURL, []*nethttp.Cookie{
		{Name: "x", Value: "1"},
	})

	tests := []struct {
		name string
		url  string
		want string
	}{
		{"default path match", "https://example.com/a/b", "x=1"},
		{"under default path", "https://example.com/a/b/other", "x=1"},
		{"parent rejected", "https://example.com/a", ""},
		{"root rejected", "https://example.com/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jar.cookiesFor(mustParseURL(t, tt.url))
			if got != tt.want {
				t.Errorf("cookiesFor(%s) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestCookieJar_MaxAgePositive(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	jar := newCookieJar()
	jar.now = fixedNow(now)
	reqURL := mustParseURL(t, "https://example.com/")

	jar.setCookies(reqURL, []*nethttp.Cookie{
		{Name: "fresh", Value: "yes", MaxAge: 3600},
	})

	// Before expiry → present.
	got := jar.cookiesFor(reqURL)
	if got != "fresh=yes" {
		t.Errorf("before expiry: cookiesFor = %q, want %q", got, "fresh=yes")
	}

	// Advance past expiry → absent.
	jar.now = fixedNow(now.Add(2 * time.Hour))
	got = jar.cookiesFor(reqURL)
	if got != "" {
		t.Errorf("after expiry: cookiesFor = %q, want %q", got, "")
	}
}

func TestCookieJar_MaxAgeNegative(t *testing.T) {
	jar := newCookieJar()
	reqURL := mustParseURL(t, "https://example.com/")

	// First set a cookie.
	jar.setCookies(reqURL, []*nethttp.Cookie{
		{Name: "old", Value: "val"},
	})
	if got := jar.cookiesFor(reqURL); got != "old=val" {
		t.Fatalf("setup: cookiesFor = %q, want %q", got, "old=val")
	}

	// Now delete it with MaxAge < 0.
	jar.setCookies(reqURL, []*nethttp.Cookie{
		{Name: "old", Value: "", MaxAge: -1},
	})
	if got := jar.cookiesFor(reqURL); got != "" {
		t.Errorf("after delete: cookiesFor = %q, want %q", got, "")
	}
}

func TestCookieJar_ExpiresInPast(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	jar := newCookieJar()
	jar.now = fixedNow(now)
	reqURL := mustParseURL(t, "https://example.com/")

	// Expires is in the past → treated as deletion, not stored.
	jar.setCookies(reqURL, []*nethttp.Cookie{
		{Name: "gone", Value: "x", Expires: now.Add(-time.Hour)},
	})

	got := jar.cookiesFor(reqURL)
	if got != "" {
		t.Errorf("expired cookie: cookiesFor = %q, want %q", got, "")
	}
}

func TestCookieJar_MaxAgeOverridesExpires(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	jar := newCookieJar()
	jar.now = fixedNow(now)
	reqURL := mustParseURL(t, "https://example.com/")

	// Expires is in the past, but Max-Age is positive → Max-Age wins.
	jar.setCookies(reqURL, []*nethttp.Cookie{
		{Name: "token", Value: "abc", MaxAge: 7200, Expires: now.Add(-time.Hour)},
	})

	got := jar.cookiesFor(reqURL)
	if got != "token=abc" {
		t.Errorf("MaxAge should override Expires: cookiesFor = %q, want %q", got, "token=abc")
	}

	// Advance past Max-Age expiry → absent.
	jar.now = fixedNow(now.Add(3 * time.Hour))
	got = jar.cookiesFor(reqURL)
	if got != "" {
		t.Errorf("after MaxAge expiry: cookiesFor = %q, want %q", got, "")
	}
}

func TestCookieJar_Replacement(t *testing.T) {
	jar := newCookieJar()
	reqURL := mustParseURL(t, "https://example.com/")

	jar.setCookies(reqURL, []*nethttp.Cookie{
		{Name: "k", Value: "old"},
	})
	jar.setCookies(reqURL, []*nethttp.Cookie{
		{Name: "k", Value: "new"},
	})

	got := jar.cookiesFor(reqURL)
	if got != "k=new" {
		t.Errorf("after replacement: cookiesFor = %q, want %q", got, "k=new")
	}
}

func TestCookieJar_DomainMismatchRejected(t *testing.T) {
	jar := newCookieJar()
	reqURL := mustParseURL(t, "https://example.com/")

	// Setting a cookie for a completely different domain should be rejected.
	jar.setCookies(reqURL, []*nethttp.Cookie{
		{Name: "evil", Value: "x", Domain: "attacker.com"},
	})

	got := jar.cookiesFor(mustParseURL(t, "https://attacker.com/"))
	if got != "" {
		t.Errorf("domain mismatch should be rejected: cookiesFor = %q, want %q", got, "")
	}
}

func TestCookieJar_EmptyNameSkipped(t *testing.T) {
	jar := newCookieJar()
	reqURL := mustParseURL(t, "https://example.com/")

	jar.setCookies(reqURL, []*nethttp.Cookie{
		{Name: "", Value: "no-name"},
		{Name: "valid", Value: "yes"},
	})

	got := jar.cookiesFor(reqURL)
	if got != "valid=yes" {
		t.Errorf("empty name should be skipped: cookiesFor = %q, want %q", got, "valid=yes")
	}
}

func TestCookieJar_SessionCookie(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	jar := newCookieJar()
	jar.now = fixedNow(now)
	reqURL := mustParseURL(t, "https://example.com/")

	// No Max-Age, no Expires → session cookie, never expires within the jar's lifetime.
	jar.setCookies(reqURL, []*nethttp.Cookie{
		{Name: "session", Value: "data"},
	})

	// Even after advancing time significantly, session cookies remain.
	jar.now = fixedNow(now.Add(365 * 24 * time.Hour))
	got := jar.cookiesFor(reqURL)
	if got != "session=data" {
		t.Errorf("session cookie should persist: cookiesFor = %q, want %q", got, "session=data")
	}
}

func TestCookieJar_PathWithTrailingSlash(t *testing.T) {
	jar := newCookieJar()
	reqURL := mustParseURL(t, "https://example.com/")

	jar.setCookies(reqURL, []*nethttp.Cookie{
		{Name: "root", Value: "yes", Path: "/"},
	})

	tests := []struct {
		name string
		url  string
		want string
	}{
		{"root", "https://example.com/", "root=yes"},
		{"any path", "https://example.com/anything", "root=yes"},
		{"deep path", "https://example.com/a/b/c", "root=yes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jar.cookiesFor(mustParseURL(t, tt.url))
			if got != tt.want {
				t.Errorf("cookiesFor(%s) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// --- helper function tests ---

func TestDomainMatches(t *testing.T) {
	tests := []struct {
		host   string
		domain string
		want   bool
	}{
		{"example.com", "example.com", true},
		{"sub.example.com", "example.com", true},
		{"deep.sub.example.com", "example.com", true},
		{"other.com", "example.com", false},
		{"notexample.com", "example.com", false},
		// IP addresses should not subdomain-match.
		{"192.168.1.1", "168.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.host+"_vs_"+tt.domain, func(t *testing.T) {
			got := domainMatches(tt.host, tt.domain)
			if got != tt.want {
				t.Errorf("domainMatches(%q, %q) = %v, want %v", tt.host, tt.domain, got, tt.want)
			}
		})
	}
}

func TestPathMatches(t *testing.T) {
	tests := []struct {
		reqPath    string
		cookiePath string
		want       bool
	}{
		{"/", "/", true},
		{"/app", "/app", true},
		{"/app/page", "/app", true},
		{"/app/page", "/app/", true},
		{"/application", "/app", false},
		{"/other", "/app", false},
		{"/", "/app", false},
	}

	for _, tt := range tests {
		t.Run(tt.reqPath+"_vs_"+tt.cookiePath, func(t *testing.T) {
			got := pathMatches(tt.reqPath, tt.cookiePath)
			if got != tt.want {
				t.Errorf("pathMatches(%q, %q) = %v, want %v", tt.reqPath, tt.cookiePath, got, tt.want)
			}
		})
	}
}

func TestDefaultPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/", "/"},
		{"/a/b/c", "/a/b"},
		{"/a", "/"},
		{"", "/"},
		{"noslash", "/"},
		{"/a/b/", "/a/b"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := defaultPath(tt.input)
			if got != tt.want {
				t.Errorf("defaultPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
