package main

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var dashboardScriptRE = regexp.MustCompile(`(?s)<script>\s*(.*?)\s*</script>`)

func extractDashboardJavaScript(html []byte) (string, error) {
	m := dashboardScriptRE.FindSubmatch(html)
	if len(m) < 2 {
		return "", fs.ErrNotExist
	}
	return string(bytes.TrimSpace(m[1])), nil
}

func readEmbeddedDashboardHTML(t *testing.T) []byte {
	t.Helper()
	data, err := dashboardFS.ReadFile("web/dashboard/index.html")
	if err != nil {
		t.Fatalf("read embedded dashboard: %v", err)
	}
	return data
}

func TestDashboardEmbeddedHTMLHasCriticalUI(t *testing.T) {
	html := string(readEmbeddedDashboardHTML(t))
	required := []string{
		`id="graph-matrix"`,
		`id="mailbox-list"`,
		`id="workspace"`,
		`id="detail-panel"`,
		`id="detail-close"`,
		`id="detail-tabs"`,
		`id="stats"`,
		`id="lang-seg"`,
		`id="login-btn"`,
		`id="refresh-btn"`,
		`id="detail-content"`,
		`collectDeliveryPeers`,
		`renderTabInbox`,
		`directedDeliveryStatus`,
		`matrixAxisLabel`,
		`parseSearchQuery`,
		`mailboxMatchesSearch`,
		`id="search-hint"`,
	}
	for _, needle := range required {
		if !strings.Contains(html, needle) {
			t.Fatalf("dashboard HTML missing %q", needle)
		}
	}
}

func TestDashboardJavaScriptSyntax(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed; skipping dashboard JavaScript syntax check")
	}

	html := readEmbeddedDashboardHTML(t)
	script, err := extractDashboardJavaScript(html)
	if err != nil {
		t.Fatalf("extract dashboard script: %v", err)
	}
	if script == "" {
		t.Fatal("dashboard script is empty")
	}

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "dashboard.js")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("write temp script: %v", err)
	}

	cmd := exec.Command("node", "--check", scriptPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node --check failed: %v\n%s", err, out)
	}
}

func TestDashboardJavaScriptResolvesReverseLinkPairs(t *testing.T) {
	html := readEmbeddedDashboardHTML(t)
	script, err := extractDashboardJavaScript(html)
	if err != nil {
		t.Fatalf("extract dashboard script: %v", err)
	}
	if !strings.Contains(script, "l.from === to && l.to === from") {
		t.Fatal("dashboard must resolve delivery via reverse link pair (from/to stored once per mailbox pair)")
	}
	if !strings.Contains(script, "reverse.reverse_status") {
		t.Fatal("dashboard must use reverse_status when matrix row/col order differs from link storage")
	}
}

func TestDashboardStatsUseDigitDiffNotFullCountAnimation(t *testing.T) {
	html := readEmbeddedDashboardHTML(t)
	script, err := extractDashboardJavaScript(html)
	if err != nil {
		t.Fatalf("extract dashboard script: %v", err)
	}
	if !strings.Contains(script, "function updateDigitText") {
		t.Fatal("dashboard must update KPI values with per-digit diff (updateDigitText)")
	}
	if !strings.Contains(script, "statsEl.dataset.initialized") {
		t.Fatal("dashboard must preserve KPI DOM across refreshes (statsEl.dataset.initialized)")
	}
	if strings.Contains(script, "function animateCount") {
		t.Fatal("dashboard must not use full-number animateCount on refresh")
	}
}

func TestDashboardJavaScriptSupportsRegexMailboxSearch(t *testing.T) {
	html := readEmbeddedDashboardHTML(t)
	script, err := extractDashboardJavaScript(html)
	if err != nil {
		t.Fatalf("extract dashboard script: %v", err)
	}
	for _, needle := range []string{
		"function parseSearchQuery",
		"function mailboxMatchesSearch",
		"function emailDomain",
		`trimmed.startsWith("/")`,
	} {
		if !strings.Contains(script, needle) {
			t.Fatalf("dashboard search missing %q", needle)
		}
	}
}

func TestDashboardJavaScriptRejectsIllegalContinueInForEach(t *testing.T) {
	html := readEmbeddedDashboardHTML(t)
	script, err := extractDashboardJavaScript(html)
	if err != nil {
		t.Fatalf("extract dashboard script: %v", err)
	}
	if regexp.MustCompile(`forEach\s*\([^)]*\)\s*\{[^}]*\bcontinue\b`).MatchString(script) {
		t.Fatal("dashboard script uses continue inside forEach; use return instead (breaks entire script parse)")
	}
}

func TestDashboardJavaScriptSyntaxFromRepoFile(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	path := filepath.Join(repoRoot(t), "cmd", "agentpost", "web", "dashboard", "index.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dashboard file: %v", err)
	}
	script, err := extractDashboardJavaScript(data)
	if err != nil {
		t.Fatalf("extract script: %v", err)
	}
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "dashboard.js")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("write temp script: %v", err)
	}
	cmd := exec.Command("node", "--check", scriptPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node --check on repo file failed: %v\n%s", err, out)
	}
}
