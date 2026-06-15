//go:build integration

package integration

import (
	"encoding/json"
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

// cliJSON runs the greet CLI, asserts exit code 0, and parses JSON output.
func cliJSON(t *testing.T, args ...string) map[string]interface{} {
	t.Helper()
	stdout, stderr, code := cliOutput(args...)
	if code != 0 {
		t.Fatalf("greet %v exited with code %d\nstdout: %s\nstderr: %s", args, code, stdout, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nraw stdout: %s", err, stdout)
	}
	return result
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

	result := cliJSON(t, "tcp", "localhost:15432")

	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result["success"])
	}
	if result["protocol"] != "tcp" {
		t.Errorf("expected protocol=tcp, got %v", result["protocol"])
	}
	if result["transport"] != "tcp" {
		t.Errorf("expected transport=tcp, got %v", result["transport"])
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
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		if result["protocol"] != "udp" {
			t.Errorf("expected protocol=udp, got %v", result["protocol"])
		}
	}
}

// ---- SSH ----

func TestIntegrationSSH(t *testing.T) {
	if err := waitForPort("localhost:2222", 60*time.Second); err != nil {
		t.Skipf("SSH container not ready: %v", err)
	}

	result := cliJSON(t, "ssh", "localhost:2222")

	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result["success"])
	}
	if result["protocol"] != "ssh" {
		t.Errorf("expected protocol=ssh, got %v", result["protocol"])
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be a map, got %T", result["data"])
	}
	versionStr, ok := data["version_string"].(string)
	if !ok {
		t.Fatalf("expected data.version_string to be a string, got %T", data["version_string"])
	}
	if !strings.HasPrefix(versionStr, "SSH-") {
		t.Errorf("expected version_string to start with SSH-, got %q", versionStr)
	}
}

// ---- PostgreSQL ----

func TestIntegrationPostgreSQL(t *testing.T) {
	if err := waitForPort("localhost:15432", 30*time.Second); err != nil {
		t.Skipf("PostgreSQL container not ready: %v", err)
	}

	result := cliJSON(t, "postgresql", "localhost:15432")

	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result["success"])
	}
	if result["protocol"] != "postgresql" {
		t.Errorf("expected protocol=postgresql, got %v", result["protocol"])
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be a map, got %T", result["data"])
	}
	// PostgreSQL alpine image does not enable SSL by default
	if data["ssl_supported"] != false {
		t.Errorf("expected ssl_supported=false, got %v", data["ssl_supported"])
	}
}

func TestIntegrationPostgreSQLDisableSSL(t *testing.T) {
	if err := waitForPort("localhost:15432", 30*time.Second); err != nil {
		t.Skipf("PostgreSQL container not ready: %v", err)
	}

	result := cliJSON(t, "postgresql", "localhost:15432", "--sslmode=disable")

	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result["success"])
	}
	if result["protocol"] != "postgresql" {
		t.Errorf("expected protocol=postgresql, got %v", result["protocol"])
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be a map, got %T", result["data"])
	}
	if data["ssl_supported"] != false {
		t.Errorf("expected ssl_supported=false with --sslmode=disable, got %v", data["ssl_supported"])
	}
}

// ---- Minecraft ----

func TestIntegrationMinecraft(t *testing.T) {
	if err := waitForPort("localhost:25565", 120*time.Second); err != nil {
		t.Skipf("Minecraft container not ready: %v", err)
	}

	result := cliJSON(t, "minecraft", "localhost:25565")

	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result["success"])
	}
	if result["protocol"] != "minecraft" {
		t.Errorf("expected protocol=minecraft, got %v", result["protocol"])
	}
	if result["transport"] != "tcp" {
		t.Errorf("expected transport=tcp, got %v", result["transport"])
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be a map, got %T", result["data"])
	}
	if _, ok := data["version"].(string); !ok {
		t.Errorf("expected data.version to be a string, got %T", data["version"])
	}
	if _, ok := data["motd"]; !ok {
		t.Errorf("expected data.motd to be present")
	}
	if _, ok := data["players_max"].(float64); !ok {
		t.Errorf("expected data.players_max to be a number, got %T", data["players_max"])
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
