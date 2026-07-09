//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var cliPath string

func TestMain(m *testing.M) {
	// Build the CLI binary
	tmpDir, err := os.MkdirTemp("", "greet-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	cliPath = filepath.Join(tmpDir, "greet")
	if os.PathSeparator == '\\' {
		cliPath += ".exe"
	}

	build := exec.Command("go", "build", "-o", cliPath, "github.com/crystade/greet/cmd/greet")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build CLI: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()
	os.Exit(code)
}

// cliOutput runs the greet CLI and returns stdout, stderr, and exit code.
func cliOutput(args ...string) (stdout, stderr string, exitCode int) {
	cmd := exec.Command(cliPath, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// cliLayer holds one parsed layer block: its per-layer timing fields
// (TTFB, TTLB, Success) plus any data-printer fields (e.g. "Version String").
type cliLayer struct {
	name   string
	fields map[string]string
}

// cliResult is the parsed layered CLI output: top-level fields plus the
// ordered list of layer blocks (transport first, then app layers).
type cliResult struct {
	top    map[string]string
	layers []cliLayer
}

// layer returns the parsed layer block with the given name.
func (r cliResult) layer(name string) (cliLayer, bool) {
	for _, l := range r.layers {
		if l.name == name {
			return l, true
		}
	}
	return cliLayer{}, false
}

// data searches every layer for a data-printer field (e.g. "SSL Supported").
func (r cliResult) data(key string) (string, bool) {
	for _, l := range r.layers {
		if v, ok := l.fields[key]; ok {
			return v, true
		}
	}
	return "", false
}

// parseResult parses the layered CLI output. Lines before the first "[layer]"
// header are top-level fields; a header like "[tcp]" starts a layer block, and
// subsequent "Key: Value" lines (indented timings or unindented data-printer
// output) attach to the most recent layer. Certificate-chain entries such as
// "[0] (leaf)" are not treated as headers because they do not end with "]".
func parseResult(output string) cliResult {
	res := cliResult{top: map[string]string{}}
	var cur *cliLayer
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			res.layers = append(res.layers, cliLayer{
				name:   strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"),
				fields: map[string]string{},
			})
			cur = &res.layers[len(res.layers)-1]
			continue
		}
		idx := strings.Index(trimmed, ": ")
		if idx < 0 {
			continue
		}
		key := trimmed[:idx]
		val := strings.TrimSpace(trimmed[idx+2:])
		if cur == nil {
			res.top[key] = val
		} else {
			cur.fields[key] = val
		}
	}
	return res
}

// cliRun runs the greet CLI, asserts exit code 0, and parses the layered output.
func cliRun(t *testing.T, args ...string) cliResult {
	t.Helper()
	stdout, stderr, code := cliOutput(args...)
	if code != 0 {
		t.Fatalf("greet %v exited with code %d\nstdout: %s\nstderr: %s", args, code, stdout, stderr)
	}
	return parseResult(stdout)
}

// parseDuration parses a time.Duration string from the CLI output.
func parseDuration(t *testing.T, s string) time.Duration {
	t.Helper()
	d, err := time.ParseDuration(s)
	if err != nil {
		t.Fatalf("failed to parse duration %q: %v", s, err)
	}
	return d
}

// assertTiming verifies the layered timing model shares a single anchor:
// top-level TTDR <= RTT, and every layer's TTFB <= TTLB while remaining
// monotonically non-decreasing down the stack (each layer's TTFB >= RTT and
// >= the previous layer's TTLB). All values must be present and non-negative.
func assertTiming(t *testing.T, result cliResult) {
	t.Helper()

	ttdr := parseDuration(t, result.top["TTDR"])
	rtt := parseDuration(t, result.top["RTT"])

	if ttdr < 0 {
		t.Errorf("expected TTDR to be non-negative, got %v", ttdr)
	}
	if rtt < 0 {
		t.Errorf("expected RTT to be non-negative, got %v", rtt)
	}
	if ttdr > rtt {
		t.Errorf("TTDR (%v) should be <= RTT (%v)", ttdr, rtt)
	}

	if len(result.layers) == 0 {
		t.Fatal("expected at least one layer in output")
	}

	prev := rtt
	for _, l := range result.layers {
		ttfbStr, ok := l.fields["TTFB"]
		if !ok {
			t.Errorf("layer %q missing TTFB", l.name)
			continue
		}
		ttlbStr, ok := l.fields["TTLB"]
		if !ok {
			t.Errorf("layer %q missing TTLB", l.name)
			continue
		}
		ttfb := parseDuration(t, ttfbStr)
		ttlb := parseDuration(t, ttlbStr)

		if ttfb < 0 || ttlb < 0 {
			t.Errorf("layer %q timings should be non-negative, got TTFB=%v TTLB=%v", l.name, ttfb, ttlb)
		}
		if ttfb > ttlb {
			t.Errorf("layer %q: TTFB (%v) should be <= TTLB (%v)", l.name, ttfb, ttlb)
		}
		if ttfb < prev {
			t.Errorf("layer %q: TTFB (%v) should be >= previous layer's end (%v)", l.name, ttfb, prev)
		}
		t.Logf("layer %q: TTFB=%v TTLB=%v", l.name, ttfb, ttlb)
		prev = ttlb
	}

	t.Logf("Timing: TTDR=%v RTT=%v", ttdr, rtt)
}

// ---- TCP generic ----

func TestIntegrationTCP(t *testing.T) {
	if err := waitForPort("localhost:15432", 30*time.Second); err != nil {
		t.Skipf("PostgreSQL container not ready (used as TCP target): %v", err)
	}

	result := cliRun(t, "tcp", "localhost:15432")

	if result.top["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result.top["Success"])
	}
	if result.top["Protocol"] != "tcp" {
		t.Errorf("expected Protocol=tcp, got %v", result.top["Protocol"])
	}
	if result.top["Transport"] != "tcp" {
		t.Errorf("expected Transport=tcp, got %v", result.top["Transport"])
	}
	if _, ok := result.layer("tcp"); !ok {
		t.Errorf("expected a [tcp] transport layer in output")
	}

	assertTiming(t, result)
}

// ---- UDP generic ----

func TestIntegrationUDP(t *testing.T) {
	// UDP against a DNS server — public but fast
	stdout, stderr, code := cliOutput("udp", "1.1.1.1:53")
	t.Logf("udp stdout: %s", stdout)
	t.Logf("udp stderr: %s", stderr)

	// UDP may succeed or fail depending on network; just verify it doesn't panic
	if code == 0 {
		result := parseResult(stdout)
		if result.top["Protocol"] != "udp" {
			t.Errorf("expected Protocol=udp, got %v", result.top["Protocol"])
		}
		if _, ok := result.layer("udp"); !ok {
			t.Errorf("expected a [udp] transport layer in output")
		}
		assertTiming(t, result)
	}
}

// ---- SSH ----

func TestIntegrationSSH(t *testing.T) {
	if err := waitForPort("localhost:2222", 60*time.Second); err != nil {
		t.Skipf("SSH container not ready: %v", err)
	}

	result := cliRun(t, "ssh", "localhost:2222")

	if result.top["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result.top["Success"])
	}
	if result.top["Protocol"] != "ssh" {
		t.Errorf("expected Protocol=ssh, got %v", result.top["Protocol"])
	}
	versionStr, _ := result.data("Version String")
	if !strings.HasPrefix(versionStr, "SSH-") {
		t.Errorf("expected Version String to start with SSH-, got %q", versionStr)
	}

	assertTiming(t, result)
}

// ---- PostgreSQL ----

func TestIntegrationPostgreSQL(t *testing.T) {
	if err := waitForPort("localhost:15432", 30*time.Second); err != nil {
		t.Skipf("PostgreSQL container not ready: %v", err)
	}

	result := cliRun(t, "postgresql", "localhost:15432")

	if result.top["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result.top["Success"])
	}
	if result.top["Protocol"] != "postgresql" {
		t.Errorf("expected Protocol=postgresql, got %v", result.top["Protocol"])
	}
	// PostgreSQL alpine image does not enable SSL by default
	if ssl, _ := result.data("SSL Supported"); ssl != "false" {
		t.Errorf("expected SSL Supported=false, got %v", ssl)
	}

	assertTiming(t, result)
}

func TestIntegrationPostgreSQLDisableSSL(t *testing.T) {
	if err := waitForPort("localhost:15432", 30*time.Second); err != nil {
		t.Skipf("PostgreSQL container not ready: %v", err)
	}

	result := cliRun(t, "postgresql", "localhost:15432", "--sslmode=disable")

	if result.top["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result.top["Success"])
	}
	if result.top["Protocol"] != "postgresql" {
		t.Errorf("expected Protocol=postgresql, got %v", result.top["Protocol"])
	}
	if ssl, _ := result.data("SSL Supported"); ssl != "false" {
		t.Errorf("expected SSL Supported=false with --sslmode=disable, got %v", ssl)
	}

	assertTiming(t, result)
}

// ---- Minecraft ----

func TestIntegrationMinecraft(t *testing.T) {
	if err := waitForPort("localhost:25565", 120*time.Second); err != nil {
		t.Skipf("Minecraft container not ready: %v", err)
	}

	result := cliRun(t, "minecraft", "localhost:25565")

	if result.top["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result.top["Success"])
	}
	if result.top["Protocol"] != "minecraft" {
		t.Errorf("expected Protocol=minecraft, got %v", result.top["Protocol"])
	}
	if result.top["Transport"] != "tcp" {
		t.Errorf("expected Transport=tcp, got %v", result.top["Transport"])
	}
	if _, ok := result.data("Version"); !ok {
		t.Errorf("expected Version to be present")
	}
	if _, ok := result.data("MOTD"); !ok {
		t.Errorf("expected MOTD to be present")
	}
	if _, ok := result.data("Players Max"); !ok {
		t.Errorf("expected Players Max to be present")
	}

	assertTiming(t, result)
}

// ---- CLI commands ----

func TestIntegrationCLIList(t *testing.T) {
	stdout, stderr, code := cliOutput("list")
	if code != 0 {
		t.Fatalf("greet list exited with code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	// list prints to stderr
	if !strings.Contains(stderr, "Available protocols") {
		t.Errorf("expected 'Available protocols' in stderr, got: %s", stderr)
	}
}

func TestIntegrationCLIUnknownProtocol(t *testing.T) {
	_, stderr, code := cliOutput("doesnotexist", "localhost:1234")
	if code == 0 {
		t.Fatal("expected non-zero exit code for unknown protocol")
	}
	if !strings.Contains(stderr, "unknown protocol") {
		t.Errorf("expected 'unknown protocol' in stderr, got: %s", stderr)
	}
}

// ---- HTTPS / HTTP ----

// httpsCase describes a single HTTPS method test against reqbin.com.
type httpsCase struct {
	method string
	path   string
}

func TestIntegrationHTTPS(t *testing.T) {
	cases := []httpsCase{
		{"GET", "/get"},
		{"POST", "/post"},
		{"PUT", "/put"},
		{"PATCH", "/patch"},
		{"DELETE", "/delete"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.method, func(t *testing.T) {
			result := cliRun(t, "https", "testserver.host",
				"--method", tc.method,
				"--path", tc.path,
			)

			if result.top["Success"] != "true" {
				t.Errorf("%s %s: expected Success=true, got %v", tc.method, tc.path, result.top["Success"])
			}
			if result.top["Protocol"] != "https" {
				t.Errorf("%s %s: expected Protocol=https, got %v", tc.method, tc.path, result.top["Protocol"])
			}
			if result.top["Transport"] != "tcp" {
				t.Errorf("%s %s: expected Transport=tcp, got %v", tc.method, tc.path, result.top["Transport"])
			}

			// Must have tcp, tls, and http layers
			if _, ok := result.layer("tcp"); !ok {
				t.Errorf("%s %s: expected [tcp] layer", tc.method, tc.path)
			}
			if _, ok := result.layer("tls"); !ok {
				t.Errorf("%s %s: expected [tls] layer", tc.method, tc.path)
			}
			httpLayer, ok := result.layer("http")
			if !ok {
				t.Fatalf("%s %s: expected [http] layer", tc.method, tc.path)
			}

			// HTTP layer must report a 200-range status
			status, ok := httpLayer.fields["Status"]
			if !ok {
				t.Errorf("%s %s: [http] layer missing Status field", tc.method, tc.path)
			} else if !strings.HasPrefix(status, "200") {
				t.Errorf("%s %s: expected HTTP 200, got %q", tc.method, tc.path, status)
			}

			// Protocol version must be present
			if protoVer, ok := httpLayer.fields["Protocol Version"]; !ok || protoVer == "" {
				t.Errorf("%s %s: [http] layer missing or empty Protocol Version", tc.method, tc.path)
			}

			assertTiming(t, result)
		})
	}
}

// ---- HTTP ----

// httpCase describes a single HTTP method test.
type httpCase struct {
	method string
	path   string
}

func TestIntegrationHTTP(t *testing.T) {
	if err := waitForHost("testserver.host:80", 10*time.Second); err != nil {
		t.Skipf("testserver.host not reachable: %v", err)
	}

	cases := []httpCase{
		{"GET", "/get"},
		{"POST", "/post"},
		{"PUT", "/put"},
		{"PATCH", "/patch"},
		{"DELETE", "/delete"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.method, func(t *testing.T) {
			result := cliRun(t, "http", "testserver.host:80",
				"--method", tc.method,
				"--path", tc.path,
			)

			if result.top["Success"] != "true" {
				t.Errorf("%s %s: expected Success=true, got %v", tc.method, tc.path, result.top["Success"])
			}
			if result.top["Protocol"] != "http" {
				t.Errorf("%s %s: expected Protocol=http, got %v", tc.method, tc.path, result.top["Protocol"])
			}
			if result.top["Transport"] != "tcp" {
				t.Errorf("%s %s: expected Transport=tcp, got %v", tc.method, tc.path, result.top["Transport"])
			}

			if _, ok := result.layer("tcp"); !ok {
				t.Errorf("%s %s: expected [tcp] layer", tc.method, tc.path)
			}
			httpLayer, ok := result.layer("http")
			if !ok {
				t.Fatalf("%s %s: expected [http] layer", tc.method, tc.path)
			}

			status, ok := httpLayer.fields["Status"]
			if !ok {
				t.Errorf("%s %s: [http] layer missing Status field", tc.method, tc.path)
			} else if !strings.HasPrefix(status, "200") {
				t.Errorf("%s %s: expected HTTP 200, got %q", tc.method, tc.path, status)
			}

			if protoVer, ok := httpLayer.fields["Protocol Version"]; !ok || protoVer == "" {
				t.Errorf("%s %s: [http] layer missing or empty Protocol Version", tc.method, tc.path)
			}

			assertTiming(t, result)
		})
	}
}

func TestIntegrationHTTPStatus(t *testing.T) {
	if err := waitForHost("testserver.host:80", 10*time.Second); err != nil {
		t.Skipf("testserver.host not reachable: %v", err)
	}

	tests := []struct {
		code     string
		expected string
	}{
		{"404", "404"},
		{"500", "500"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.code, func(t *testing.T) {
			result := cliRun(t, "http", "testserver.host:80",
				"--path", "/status/"+tt.code,
			)

			// Any HTTP response (even 4xx/5xx) results in layer success.
			if result.top["Success"] != "true" {
				t.Errorf("expected Success=true for HTTP %s, got %v", tt.code, result.top["Success"])
			}

			httpLayer, ok := result.layer("http")
			if !ok {
				t.Fatalf("expected [http] layer")
			}
			status, ok := httpLayer.fields["Status"]
			if !ok {
				t.Fatalf("[http] layer missing Status field")
			}
			if !strings.Contains(status, tt.expected) {
				t.Errorf("expected Status to contain %q, got %q", tt.expected, status)
			}

			assertTiming(t, result)
		})
	}
}

func TestIntegrationHTTPRedirect(t *testing.T) {
	if err := waitForHost("testserver.host:80", 10*time.Second); err != nil {
		t.Skipf("testserver.host not reachable: %v", err)
	}

	result := cliRun(t, "http", "testserver.host:80",
		"--path", "/redirect/3",
		"--max-redirects", "5",
	)

	if result.top["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result.top["Success"])
	}

	httpLayer, ok := result.layer("http")
	if !ok {
		t.Fatalf("expected [http] layer")
	}
	status, ok := httpLayer.fields["Status"]
	if !ok {
		t.Fatalf("[http] layer missing Status field")
	}
	if !strings.HasPrefix(status, "200") {
		t.Errorf("expected HTTP 200 after redirects, got %q", status)
	}

	if _, ok := httpLayer.fields["Location"]; !ok {
		t.Errorf("expected Location header from redirect chain in http layer fields, got: %v", httpLayer.fields)
	}

	assertTiming(t, result)
}

func TestIntegrationHTTPErrorClose(t *testing.T) {
	if err := waitForHost("testserver.host:80", 10*time.Second); err != nil {
		t.Skipf("testserver.host not reachable: %v", err)
	}

	stdout, stderr, code := cliOutput("http", "testserver.host:80",
		"--path", "/error/close",
	)

	// Connection close should result in non-zero exit code.
	if code == 0 {
		t.Fatalf("expected non-zero exit code for /error/close, got 0\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	// Should report an error
	if !strings.Contains(stderr, "Error:") {
		t.Errorf("expected error output for /error/close, got stderr: %s", stderr)
	}

	t.Logf("stdout: %s", stdout)
	t.Logf("stderr: %s", stderr)
}

func TestIntegrationHTTPErrorReset(t *testing.T) {
	if err := waitForHost("testserver.host:80", 10*time.Second); err != nil {
		t.Skipf("testserver.host not reachable: %v", err)
	}

	stdout, stderr, code := cliOutput("http", "testserver.host:80",
		"--path", "/error/reset",
	)

	// Connection reset should result in non-zero exit code.
	if code == 0 {
		t.Fatalf("expected non-zero exit code for /error/reset, got 0\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	// Should report a connection-level error
	if !strings.Contains(stderr, "Error:") {
		t.Errorf("expected error output for /error/reset, got stderr: %s", stderr)
	}

	t.Logf("stdout: %s", stdout)
	t.Logf("stderr: %s", stderr)
}

// ---- TLS scenarios ----

const tlsTestHost = "testserver.host:443"

func TestIntegrationTLSSelfSigned(t *testing.T) {
	if err := waitForHost("testserver.host:443", 10*time.Second); err != nil {
		t.Skipf("testserver.host not reachable: %v", err)
	}

	result := cliRun(t, "https", "self-signed.testserver.host")

	// TLS handshake succeeds with InsecureSkipVerify.
	if result.top["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result.top["Success"])
	}

	// Must have tcp, tls, http layers.
	assertTLSThreeLayers(t, result)

	// Chain status should NOT be just "OK" for self-signed certs.
	chainStatus, ok := result.data("Chain Status")
	if !ok {
		t.Errorf("expected Chain Status in output")
	} else if chainStatus == "OK" {
		t.Errorf("expected non-OK chain status for self-signed cert, got OK")
	} else {
		t.Logf("self-signed chain status: %s", chainStatus)
	}

	assertTiming(t, result)
}

func TestIntegrationTLSExpired(t *testing.T) {
	if err := waitForHost("testserver.host:443", 10*time.Second); err != nil {
		t.Skipf("testserver.host not reachable: %v", err)
	}

	result := cliRun(t, "https", "expired.testserver.host")

	if result.top["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result.top["Success"])
	}

	assertTLSThreeLayers(t, result)

	// Chain status should contain an expiry-related warning.
	chainStatus, ok := result.data("Chain Status")
	if !ok {
		t.Errorf("expected Chain Status in output")
	} else if !strings.Contains(chainStatus, "DATE_INVALID") && !strings.Contains(chainStatus, "expired") {
		t.Errorf("expected expiry warning in chain status, got: %s", chainStatus)
	} else {
		t.Logf("expired chain status: %s", chainStatus)
	}

	assertTiming(t, result)
}

func TestIntegrationTLSWrongHost(t *testing.T) {
	if err := waitForHost("testserver.host:443", 10*time.Second); err != nil {
		t.Skipf("testserver.host not reachable: %v", err)
	}

	result := cliRun(t, "https", "wrong-host.testserver.host")

	if result.top["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result.top["Success"])
	}

	assertTLSThreeLayers(t, result)

	// Chain status should contain a name-mismatch warning.
	chainStatus, ok := result.data("Chain Status")
	if !ok {
		t.Errorf("expected Chain Status in output")
	} else if !strings.Contains(chainStatus, "COMMON_NAME_INVALID") {
		t.Errorf("expected COMMON_NAME_INVALID in chain status, got: %s", chainStatus)
	} else {
		t.Logf("wrong-host chain status: %s", chainStatus)
	}

	assertTiming(t, result)
}

func TestIntegrationTLSUntrustedRoot(t *testing.T) {
	if err := waitForHost("testserver.host:443", 10*time.Second); err != nil {
		t.Skipf("testserver.host not reachable: %v", err)
	}

	result := cliRun(t, "https", "untrusted-root.testserver.host")

	if result.top["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result.top["Success"])
	}

	assertTLSThreeLayers(t, result)

	// Untrusted root: handshake succeeds (InsecureSkipVerify) but cert
	// may be flagged as self-signed or have other issues.
	chainStatus, ok := result.data("Chain Status")
	if !ok {
		t.Errorf("expected Chain Status in output")
	} else {
		t.Logf("untrusted-root chain status: %s", chainStatus)
	}

	assertTiming(t, result)
}

func TestIntegrationTLSRevoked(t *testing.T) {
	if err := waitForHost("testserver.host:443", 10*time.Second); err != nil {
		t.Skipf("testserver.host not reachable: %v", err)
	}

	result := cliRun(t, "https", "revoked.testserver.host")

	if result.top["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result.top["Success"])
	}

	assertTLSThreeLayers(t, result)

	// Revoked cert: handshake succeeds but should have some status flag.
	chainStatus, ok := result.data("Chain Status")
	if !ok {
		t.Errorf("expected Chain Status in output")
	} else {
		t.Logf("revoked chain status: %s", chainStatus)
	}

	assertTiming(t, result)
}

// assertTLSThreeLayers checks that the result contains tcp, tls, and http layers.
func assertTLSThreeLayers(t *testing.T, result cliResult) {
	t.Helper()
	if _, ok := result.layer("tcp"); !ok {
		t.Errorf("expected [tcp] layer")
	}
	if _, ok := result.layer("tls"); !ok {
		t.Errorf("expected [tls] layer")
	}
	if _, ok := result.layer("http"); !ok {
		t.Errorf("expected [http] layer")
	}
}

// waitForHost checks if a TCP address is reachable. Used to skip tests
// when external servers (e.g. testserver.host) are unavailable.
func waitForHost(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.Command(cliPath, "tcp", addr)
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("host %s not reachable after %v", addr, timeout)
}

// waitForPort retries a TCP connection until it succeeds or times out.
func waitForPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.Command(cliPath, "tcp", addr)
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("port %s not reachable after %v", addr, timeout)
}
