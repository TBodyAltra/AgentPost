package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		`curl -fsS -H "Authorization: Bearer test-gateway-token" https://example.domain/api/v1/skill`,
		"Localhost:  http://127.0.0.1:8080",
		"LAN:        http://192.168.1.50:8080",
		"Public IP:  http://203.0.113.10:8080",
		"Domain (HTTPS):  https://example.domain",
		"AGENTPOST_EMAIL_SUFFIX=example.domain",
		"AGENTPOST_API_TOKEN=test-gateway-token",
		"Authorization: Bearer test-gateway-token",
		"only /healthz is public",
		"Fetch the Skill document",
		"--- end prompt ---",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "# AgentPost 使用说明") || strings.Contains(prompt, "--- AgentPost Skill") {
		t.Fatalf("onboarding prompt must not embed full skill document:\n%s", prompt)
	}
	if strings.Contains(prompt, "POST /api/v1/register with your public key") {
		t.Fatalf("onboarding prompt must not duplicate API workflow; use GET /api/v1/skill:\n%s", prompt)
	}
}

func TestBuildSkillMarkdownIncludesClientAgentGuide(t *testing.T) {
	meta := skillMeta{
		ServerURL: "http://127.0.0.1:8080",
		Domain:    "agent.test",
		Language:  "zh",
	}
	body := buildSkillMarkdown(meta, "zh")
	for _, want := range []string{
		"# AgentPost 使用说明（Skill）",
		"# AgentPost client agent (platform-neutral)",
		"Fetch Skill after onboarding",
		"Non-negotiable behaviors",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("skill missing %q", want)
		}
	}
	idxDeploy := strings.Index(body, "# AgentPost 使用说明（Skill）")
	idxClient := strings.Index(body, "# AgentPost client agent")
	if idxDeploy < 0 || idxClient < 0 || idxClient <= idxDeploy {
		t.Fatalf("expected deployment skill before client guide; deploy=%d client=%d", idxDeploy, idxClient)
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
