package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestBuildAgentOnboardingPromptIncludesConnectionURLsAndToken(t *testing.T) {
	cfg := Config{
		Domain:   "example.domain",
		APIToken: "test-gateway-token",
	}
	urls := skillConnectionURLs{
		Localhost: "http://127.0.0.1:8080",
		LAN:       "http://192.168.1.50:8080",
		PublicIP:  "http://203.0.113.10:8080",
		Domain:    "https://example.domain",
	}
	prompt := buildAgentOnboardingPrompt(cfg, urls, skillExampleURL(urls, "http://fallback.test"))

	for _, want := range []string{
		"--- Agent onboarding prompt (copy below) ---",
		"--- AgentPost Skill (this deployment) ---",
		"# AgentPost 使用说明（Skill）",
		"request / reply",
		"--- end skill ---",
		`curl -fsS -H "Authorization: Bearer test-gateway-token" https://example.domain/api/v1/skill`,
		"Localhost:  http://127.0.0.1:8080",
		"LAN:        http://192.168.1.50:8080",
		"Public IP:  http://203.0.113.10:8080",
		"Domain (HTTPS):  https://example.domain",
		"AGENTPOST_EMAIL_SUFFIX=example.domain",
		"AGENTPOST_API_TOKEN=test-gateway-token",
		"Authorization: Bearer test-gateway-token",
		"except /healthz",
		"--- end prompt ---",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\n%s", want, prompt)
		}
	}
}

func TestBuildAgentOnboardingPromptOmitsTokenWhenUnset(t *testing.T) {
	cfg := Config{Domain: "agent.local", APIToken: ""}
	urls := skillConnectionURLs{Localhost: "http://127.0.0.1:8080"}
	prompt := buildAgentOnboardingPrompt(cfg, urls, urls.Localhost)
	if strings.Contains(prompt, "AGENTPOST_API_TOKEN=") {
		t.Fatalf("expected no token lines in prompt:\n%s", prompt)
	}
}

func TestBuildAgentOnboardingPromptEnglishSkill(t *testing.T) {
	t.Setenv("AGENTPOST_ONBOARDING_LANG", "en")
	cfg := Config{Domain: "agent.local", APIToken: "tok"}
	urls := skillConnectionURLs{Localhost: "http://127.0.0.1:8080"}
	prompt := buildAgentOnboardingPrompt(cfg, urls, urls.Localhost)
	if !strings.Contains(prompt, "# AgentPost Skill Guide") {
		t.Fatalf("expected English skill in prompt:\n%s", prompt)
	}
}

func TestPrintOnboardingFlag(t *testing.T) {
	t.Setenv("AGENTPOST_CONNECT_LOCALHOST", "http://127.0.0.1:19999")
	dir := t.TempDir()
	cfgPath := dir + "/config.yaml"
	if err := os.WriteFile(cfgPath, []byte("domain: flag.test\nhttp_addr: \":19999\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", "./cmd/agentpost", "-config", cfgPath, "-print-onboarding")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("print-onboarding: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "--- Agent onboarding prompt") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	if !strings.Contains(string(out), "flag.test") {
		t.Fatalf("expected domain in output:\n%s", out)
	}
}

func TestDashboardSnapshotIncludesOnboardingPrompt(t *testing.T) {
	t.Setenv("AGENTPOST_CONNECT_LOCALHOST", "http://127.0.0.1:18080")
	t.Setenv("AGENTPOST_CONNECT_LAN", "http://10.0.0.5:8080")
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		MaxMessageBytes: defaultMaxMessageBytes,
		APIToken:        "snap-token",
	})
	handler := app.routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	req.Header.Set("Authorization", "Bearer snap-token")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var snap dashboardResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.OnboardingPrompt == "" {
		t.Fatal("onboarding_prompt is empty")
	}
	if !strings.Contains(snap.OnboardingPrompt, "10.0.0.5:8080") {
		t.Fatalf("expected LAN URL in prompt:\n%s", snap.OnboardingPrompt)
	}
}
