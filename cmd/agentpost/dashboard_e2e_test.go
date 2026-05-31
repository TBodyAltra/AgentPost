package main

import (
	"bytes"
	"crypto/ed25519"
	crand "crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func runPlaywright(t *testing.T, e2eDir, baseURL string, args ...string) {
	t.Helper()
	cmd := exec.Command("npm", "test")
	if len(args) > 0 {
		cmd = exec.Command("npm", append([]string{"exec", "--", "playwright", "test"}, args...)...)
	}
	cmd.Dir = e2eDir
	cmd.Env = append(os.Environ(), "BASE_URL="+baseURL, "CI=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("playwright failed: %v\n%s", err, out)
	}
}

func e2eDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "e2e")
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "@playwright", "test")); err != nil {
		t.Skip("e2e dependencies not installed; run: npm ci --prefix e2e && npx --prefix e2e playwright install chromium")
	}
	return dir
}

func TestDashboardPlaywrightE2E(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not installed")
	}

	dir := e2eDir(t)

	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
		APIToken:        "",
	})
	handler := app.routes()

	pubA, _, _ := ed25519.GenerateKey(crand.Reader)
	pubB, _, _ := ed25519.GenerateKey(crand.Reader)
	pubG, _, _ := ed25519.GenerateKey(crand.Reader)
	registerDashboardUser(t, handler, "alpha", "agent.test", pubA, nil)
	registerDashboardUser(t, handler, "beta", "agent.test", pubB, nil)
	// gamma blocks alpha so the delivery matrix has non-delivery (empty) cells for E2E.
	registerDashboardUser(t, handler, "gamma", "agent.test", pubG, &InboxPolicy{
		Blocklist: []string{"alpha@agent.test"},
	})

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	runPlaywright(t, dir, ts.URL, "dashboard.spec.mjs")
}

func TestDashboardPlaywrightE2EWithGatewayToken(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not installed")
	}

	dir := e2eDir(t)

	const token = "e2e-gateway-token"
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
		APIToken:        token,
	})
	handler := app.routes()

	pubA, _, _ := ed25519.GenerateKey(crand.Reader)
	registerDashboardUserWithGateway(t, handler, token, "solo", "agent.test", pubA, nil)

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	cmd := exec.Command("npm", "exec", "--", "playwright", "test", "dashboard-token.spec.mjs")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"BASE_URL="+ts.URL,
		"GATEWAY_TOKEN="+token,
		"CI=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("playwright token e2e failed: %v\n%s", err, out)
	}
}

func TestDashboardE2ERegisterWithTokenUsesBearer(t *testing.T) {
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		MaxMessageBytes: defaultMaxMessageBytes,
		APIToken:        "gate",
	})
	handler := app.routes()

	pub, _, _ := ed25519.GenerateKey(crand.Reader)
	body := mustJSON(t, registerRequest{
		Username:   "x",
		Domain:     "agent.test",
		PublicKey:  hex.EncodeToString(pub),
		TTLSeconds: 3600,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer gate")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("register with bearer status = %d, body = %s", resp.Code, resp.Body.String())
	}
}
