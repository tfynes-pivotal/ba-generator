// Integration tests for ba-generator.
//
// Requires hub-api-config.json with valid credentials in the same directory.
// Tests build the binary and exercise the main flags, asserting exit 0 and
// expected output patterns.
//
// Unofficial project "as is"; no warranty. Use at your own discretion.
//
// Run with:
//   go build -o ba-generator .
//   go test -v -count=1

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const binaryName = "ba-generator"

func TestMain(m *testing.M) {
	cwd, err := os.Getwd()
	if err != nil {
		os.Exit(1)
	}
	binPath := filepath.Join(cwd, binaryName)
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		build := exec.Command("go", "build", "-o", binaryName, ".")
		build.Dir = cwd
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			os.Stderr.WriteString("build failed: run 'go build -o ba-generator .' first\n")
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

// requireConfig skips the test if hub-api-config.json is absent or incomplete.
func requireConfig(t *testing.T) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	configPath := filepath.Join(cwd, configFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Skipf("hub-api-config.json not found (%v) — skipping integration test", err)
	}
	var cfg AuthConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Skipf("hub-api-config.json parse error (%v) — skipping", err)
	}
	if cfg.OAuthAppID == "" || cfg.OAuthAppSecret == "" || cfg.GraphQLEndpoint == "" {
		t.Skip("hub-api-config.json has empty fields — skipping integration test")
	}
}

func run(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cwd, _ := os.Getwd()
	cmd := exec.Command(filepath.Join(cwd, binaryName), args...)
	cmd.Dir = cwd
	out, _ := cmd.CombinedOutput()
	return string(out), cmd.ProcessState.ExitCode()
}

// TestNoFlags verifies that running with no flags prints usage and exits non-zero.
func TestNoFlags(t *testing.T) {
	out, code := run(t)
	if code == 0 {
		t.Errorf("expected non-zero exit with no flags, got 0")
	}
	if !strings.Contains(out, "BA Generator") {
		t.Errorf("expected usage output to contain 'BA Generator', got:\n%s", out)
	}
}

// TestGenerateToken verifies -generate-token prints a non-empty token line.
func TestGenerateToken(t *testing.T) {
	requireConfig(t)
	out, code := run(t, "-generate-token")
	if code != 0 {
		t.Fatalf("-generate-token exited %d:\n%s", code, out)
	}
	token := strings.TrimSpace(out)
	if token == "" {
		t.Errorf("-generate-token produced empty output")
	}
}

// TestListSpaces verifies -list-spaces exits 0 and prints summary lines.
func TestListSpaces(t *testing.T) {
	requireConfig(t)
	out, code := run(t, "-list-spaces")
	if code != 0 {
		t.Fatalf("-list-spaces exited %d:\n%s", code, out)
	}
	if !strings.Contains(out, "Fetched") {
		t.Errorf("expected 'Fetched' in output, got:\n%s", out)
	}
}

// TestGenerateDryRun verifies -generate -dry-run exits 0 and mentions DRY RUN.
func TestGenerateDryRun(t *testing.T) {
	requireConfig(t)
	out, code := run(t, "-generate", "-dry-run")
	if code != 0 {
		t.Fatalf("-generate -dry-run exited %d:\n%s", code, out)
	}
	if !strings.Contains(out, "DRY RUN") {
		t.Errorf("expected 'DRY RUN' in output, got:\n%s", out)
	}
}

// Unit tests (no Hub connectivity required)

// TestExtractADID checks the regex extraction helper.
func TestExtractADID(t *testing.T) {
	import_re := mustCompile(`ad\d{8}`)
	cases := []struct {
		input string
		want  string
	}{
		{"myorg-ad12345678-dev", "ad12345678"},
		{"ad00000001-prod", "ad00000001"},
		{"no-match-here", ""},
		{"prefix-ad99999999-suffix", "ad99999999"},
	}
	for _, tc := range cases {
		got := extractADID(tc.input, import_re)
		if got != tc.want {
			t.Errorf("extractADID(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func mustCompile(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(err)
	}
	return re
}

// TestLoadADNameMap verifies the CSV loader reads headers and data rows.
func TestLoadADNameMap(t *testing.T) {
	f, err := os.CreateTemp("", "admap-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	f.WriteString("ad_Id,ad_Name\n")
	f.WriteString("ad00000001,My First App\n")
	f.WriteString("ad00000002,My Second App\n")
	f.Close()

	m, err := loadADNameMap(f.Name())
	if err != nil {
		t.Fatalf("loadADNameMap: %v", err)
	}
	if m["ad00000001"] != "My First App" {
		t.Errorf("expected 'My First App', got %q", m["ad00000001"])
	}
	if m["ad00000002"] != "My Second App" {
		t.Errorf("expected 'My Second App', got %q", m["ad00000002"])
	}
	if len(m) != 2 {
		t.Errorf("expected 2 entries, got %d", len(m))
	}
}
