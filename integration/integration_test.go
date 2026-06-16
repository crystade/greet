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

// parseKeyValues parses "Key: Value" lines into a map.
func parseKeyValues(output string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ": ")
		if idx < 0 {
			continue
		}
		key := line[:idx]
		val := line[idx+2:]
		result[key] = val
	}
	return result
}

// cliKV runs the greet CLI, asserts exit code 0, and parses key-value output.
func cliKV(t *testing.T, args ...string) map[string]string {
	t.Helper()
	stdout, stderr, code := cliOutput(args...)
	if code != 0 {
		t.Fatalf("greet %v exited with code %d\nstdout: %s\nstderr: %s", args, code, stdout, stderr)
	}
	return parseKeyValues(stdout)
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

// ---- TCP generic ----

func TestIntegrationTCP(t *testing.T) {
	if err := waitForPort("localhost:15432", 30*time.Second); err != nil {
		t.Skipf("PostgreSQL container not ready (used as TCP target): %v", err)
	}

	result := cliKV(t, "tcp", "localhost:15432")

	if result["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result["Success"])
	}
	if result["Protocol"] != "tcp" {
		t.Errorf("expected Protocol=tcp, got %v", result["Protocol"])
	}
	if result["Transport"] != "tcp" {
		t.Errorf("expected Transport=tcp, got %v", result["Transport"])
	}
}

// ---- UDP generic ----

func TestIntegrationUDP(t *testing.T) {
	// UDP against a DNS server — public but fast
	stdout, stderr, code := cliOutput("udp", "1.1.1.1:53")
	t.Logf("udp stdout: %s", stdout)
	t.Logf("udp stderr: %s", stderr)

	// UDP may succeed or fail depending on network; just verify it doesn't panic
	if code == 0 {
		result := parseKeyValues(stdout)
		if result["Protocol"] != "udp" {
			t.Errorf("expected Protocol=udp, got %v", result["Protocol"])
		}
	}
}

// ---- SSH ----

func TestIntegrationSSH(t *testing.T) {
	if err := waitForPort("localhost:2222", 60*time.Second); err != nil {
		t.Skipf("SSH container not ready: %v", err)
	}

	result := cliKV(t, "ssh", "localhost:2222")

	if result["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result["Success"])
	}
	if result["Protocol"] != "ssh" {
		t.Errorf("expected Protocol=ssh, got %v", result["Protocol"])
	}
	versionStr := result["Version String"]
	if !strings.HasPrefix(versionStr, "SSH-") {
		t.Errorf("expected Version String to start with SSH-, got %q", versionStr)
	}
}

// ---- PostgreSQL ----

func TestIntegrationPostgreSQL(t *testing.T) {
	if err := waitForPort("localhost:15432", 30*time.Second); err != nil {
		t.Skipf("PostgreSQL container not ready: %v", err)
	}

	result := cliKV(t, "postgresql", "localhost:15432")

	if result["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result["Success"])
	}
	if result["Protocol"] != "postgresql" {
		t.Errorf("expected Protocol=postgresql, got %v", result["Protocol"])
	}
	// PostgreSQL alpine image does not enable SSL by default
	if result["SSL Supported"] != "false" {
		t.Errorf("expected SSL Supported=false, got %v", result["SSL Supported"])
	}
}

func TestIntegrationPostgreSQLDisableSSL(t *testing.T) {
	if err := waitForPort("localhost:15432", 30*time.Second); err != nil {
		t.Skipf("PostgreSQL container not ready: %v", err)
	}

	result := cliKV(t, "postgresql", "localhost:15432", "--sslmode=disable")

	if result["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result["Success"])
	}
	if result["Protocol"] != "postgresql" {
		t.Errorf("expected Protocol=postgresql, got %v", result["Protocol"])
	}
	if result["SSL Supported"] != "false" {
		t.Errorf("expected SSL Supported=false with --sslmode=disable, got %v", result["SSL Supported"])
	}
}

// ---- Minecraft ----

func TestIntegrationMinecraft(t *testing.T) {
	if err := waitForPort("localhost:25565", 120*time.Second); err != nil {
		t.Skipf("Minecraft container not ready: %v", err)
	}

	result := cliKV(t, "minecraft", "localhost:25565")

	if result["Success"] != "true" {
		t.Errorf("expected Success=true, got %v", result["Success"])
	}
	if result["Protocol"] != "minecraft" {
		t.Errorf("expected Protocol=minecraft, got %v", result["Protocol"])
	}
	if result["Transport"] != "tcp" {
		t.Errorf("expected Transport=tcp, got %v", result["Transport"])
	}
	if _, ok := result["Version"]; !ok {
		t.Errorf("expected Version to be present")
	}
	if _, ok := result["MOTD"]; !ok {
		t.Errorf("expected MOTD to be present")
	}
	if _, ok := result["Players Max"]; !ok {
		t.Errorf("expected Players Max to be present")
	}
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
