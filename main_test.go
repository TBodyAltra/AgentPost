package main

import (
	"bytes"
	"crypto/ed25519"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRegisterSendAndPoll(t *testing.T) {
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	handler := app.routes()

	publicKey, privateKey, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	registerBody := mustJSON(t, registerRequest{
		Username:   "bot_1",
		PublicKey:  hex.EncodeToString(publicKey),
		TTLSeconds: 3600,
	})
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerResp := httptest.NewRecorder()
	handler.ServeHTTP(registerResp, registerReq)
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", registerResp.Code, registerResp.Body.String())
	}

	sendBody := mustJSON(t, sendRequest{
		To:      "bot_1@agent.test",
		Subject: "hello",
		Body:    "internal delivery works",
	})
	sendReq := signedRequest(t, http.MethodPost, "/api/v1/send", sendBody, "bot_1@agent.test", privateKey)
	sendReq.Header.Set("Content-Type", "application/json")
	sendResp := httptest.NewRecorder()
	handler.ServeHTTP(sendResp, sendReq)
	if sendResp.Code != http.StatusOK {
		t.Fatalf("send status = %d, body = %s", sendResp.Code, sendResp.Body.String())
	}

	pollReq := signedRequest(t, http.MethodGet, "/api/v1/messages", nil, "bot_1@agent.test", privateKey)
	pollResp := httptest.NewRecorder()
	handler.ServeHTTP(pollResp, pollReq)
	if pollResp.Code != http.StatusOK {
		t.Fatalf("poll status = %d, body = %s", pollResp.Code, pollResp.Body.String())
	}

	var got messagesResponse
	if err := json.NewDecoder(pollResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode poll response: %v", err)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(got.Messages))
	}
	if got.Messages[0].Subject != "hello" || got.Messages[0].BodyText != "internal delivery works" {
		t.Fatalf("unexpected message: %+v", got.Messages[0])
	}
}

func TestMessagesPollIsDestructive(t *testing.T) {
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	handler := app.routes()

	publicKey, privateKey, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	registerBody := mustJSON(t, registerRequest{
		Username:   "bot_1",
		PublicKey:  hex.EncodeToString(publicKey),
		TTLSeconds: 3600,
	})
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerResp := httptest.NewRecorder()
	handler.ServeHTTP(registerResp, registerReq)
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", registerResp.Code, registerResp.Body.String())
	}

	sendBody := mustJSON(t, sendRequest{
		To:      "bot_1@agent.test",
		Subject: "destructive poll",
		Body:    "read once",
	})
	sendReq := signedRequest(t, http.MethodPost, "/api/v1/send", sendBody, "bot_1@agent.test", privateKey)
	sendReq.Header.Set("Content-Type", "application/json")
	sendResp := httptest.NewRecorder()
	handler.ServeHTTP(sendResp, sendReq)
	if sendResp.Code != http.StatusOK {
		t.Fatalf("send status = %d, body = %s", sendResp.Code, sendResp.Body.String())
	}

	poll := func() messagesResponse {
		t.Helper()
		req := signedRequest(t, http.MethodGet, "/api/v1/messages", nil, "bot_1@agent.test", privateKey)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("poll status = %d, body = %s", resp.Code, resp.Body.String())
		}
		var got messagesResponse
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode poll response: %v", err)
		}
		return got
	}

	if got := poll(); len(got.Messages) != 1 {
		t.Fatalf("first poll message count = %d, want 1", len(got.Messages))
	}
	if got := poll(); len(got.Messages) != 0 {
		t.Fatalf("second poll message count = %d, want 0", len(got.Messages))
	}
}

func TestRegisterProfileDirectoryAndUnregister(t *testing.T) {
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	handler := app.routes()

	pubA, privA, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key A: %v", err)
	}
	pubB, privB, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key B: %v", err)
	}

	registerA := mustJSON(t, registerRequest{
		Username:   "bot_a",
		PublicKey:  hex.EncodeToString(pubA),
		TTLSeconds: 3600,
		Profile: &AgentProfile{
			DisplayName:      "Agent Alpha",
			Host:             "worker-01",
			Responsibilities: "research",
			Skills:           []string{"summarize", "search"},
			MCPServices:      []string{"filesystem"},
			Capabilities:     []string{"can read PDFs"},
			Notes:            "primary researcher",
		},
	})
	reqA := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(registerA))
	reqA.Header.Set("Content-Type", "application/json")
	respA := httptest.NewRecorder()
	handler.ServeHTTP(respA, reqA)
	if respA.Code != http.StatusCreated {
		t.Fatalf("register A status = %d, body = %s", respA.Code, respA.Body.String())
	}

	registerB := mustJSON(t, registerRequest{
		Username:   "bot_b",
		PublicKey:  hex.EncodeToString(pubB),
		TTLSeconds: 3600,
		Profile: &AgentProfile{
			DisplayName: "Agent Beta",
			Host:        "worker-02",
		},
	})
	reqB := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(registerB))
	reqB.Header.Set("Content-Type", "application/json")
	respB := httptest.NewRecorder()
	handler.ServeHTTP(respB, reqB)
	if respB.Code != http.StatusCreated {
		t.Fatalf("register B status = %d, body = %s", respB.Code, respB.Body.String())
	}

	listReq := signedRequest(t, http.MethodGet, "/api/v1/agents", nil, "bot_a@agent.test", privA)
	listResp := httptest.NewRecorder()
	handler.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("agents status = %d, body = %s", listResp.Code, listResp.Body.String())
	}

	var directory agentsResponse
	if err := json.NewDecoder(listResp.Body).Decode(&directory); err != nil {
		t.Fatalf("decode agents response: %v", err)
	}
	if len(directory.Agents) != 2 {
		t.Fatalf("agent count = %d, want 2", len(directory.Agents))
	}

	var alpha *agentEntry
	for i := range directory.Agents {
		if directory.Agents[i].Username == "bot_a" {
			alpha = &directory.Agents[i]
			break
		}
	}
	if alpha == nil {
		t.Fatalf("bot_a not found in directory: %+v", directory.Agents)
	}
	if alpha.Profile.DisplayName != "Agent Alpha" || alpha.Profile.Host != "worker-01" {
		t.Fatalf("unexpected profile: %+v", alpha.Profile)
	}
	if len(alpha.Profile.Skills) != 2 || alpha.Profile.MCPServices[0] != "filesystem" {
		t.Fatalf("unexpected profile lists: %+v", alpha.Profile)
	}

	sendBody := mustJSON(t, sendRequest{
		To:      "bot_a@agent.test",
		Subject: "queued",
		Body:    "should be deleted on unregister",
	})
	sendReq := signedRequest(t, http.MethodPost, "/api/v1/send", sendBody, "bot_b@agent.test", privB)
	sendReq.Header.Set("Content-Type", "application/json")
	sendResp := httptest.NewRecorder()
	handler.ServeHTTP(sendResp, sendReq)
	if sendResp.Code != http.StatusOK {
		t.Fatalf("send status = %d, body = %s", sendResp.Code, sendResp.Body.String())
	}

	delReq := signedRequest(t, http.MethodDelete, "/api/v1/account", nil, "bot_a@agent.test", privA)
	delResp := httptest.NewRecorder()
	handler.ServeHTTP(delResp, delReq)
	if delResp.Code != http.StatusOK {
		t.Fatalf("unregister status = %d, body = %s", delResp.Code, delResp.Body.String())
	}

	listAfter := signedRequest(t, http.MethodGet, "/api/v1/agents", nil, "bot_b@agent.test", privB)
	listAfterResp := httptest.NewRecorder()
	handler.ServeHTTP(listAfterResp, listAfter)
	if listAfterResp.Code != http.StatusOK {
		t.Fatalf("agents after unregister status = %d", listAfterResp.Code)
	}
	if err := json.NewDecoder(listAfterResp.Body).Decode(&directory); err != nil {
		t.Fatalf("decode agents response: %v", err)
	}
	if len(directory.Agents) != 1 || directory.Agents[0].Username != "bot_b" {
		t.Fatalf("expected only bot_b after unregister, got %+v", directory.Agents)
	}

	sendAfter := signedRequest(t, http.MethodPost, "/api/v1/send", sendBody, "bot_b@agent.test", privB)
	sendAfter.Header.Set("Content-Type", "application/json")
	sendAfterResp := httptest.NewRecorder()
	handler.ServeHTTP(sendAfterResp, sendAfter)
	if sendAfterResp.Code != http.StatusNotFound {
		t.Fatalf("send to unregistered bot_a status = %d, want 404", sendAfterResp.Code)
	}
}

func TestRegisterCapsTTLAndUsesDefaultTTL(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	app.now = func() time.Time { return now }
	handler := app.routes()

	publicKey, _, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	register := func(username string, ttl int64) registerResponse {
		t.Helper()
		body := mustJSON(t, registerRequest{
			Username:   username,
			PublicKey:  hex.EncodeToString(publicKey),
			TTLSeconds: ttl,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("register %s status = %d, body = %s", username, resp.Code, resp.Body.String())
		}
		var got registerResponse
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode register response: %v", err)
		}
		return got
	}

	capped := register("ttl_capped", maxTTLSeconds+3600)
	if !capped.ExpiresAt.Equal(now.Add(time.Duration(maxTTLSeconds) * time.Second)) {
		t.Fatalf("capped expires_at = %s, want %s", capped.ExpiresAt, now.Add(time.Duration(maxTTLSeconds)*time.Second))
	}

	defaulted := register("ttl_default", 0)
	if !defaulted.ExpiresAt.Equal(now.Add(time.Duration(defaultTTLSeconds) * time.Second)) {
		t.Fatalf("default expires_at = %s, want %s", defaulted.ExpiresAt, now.Add(time.Duration(defaultTTLSeconds)*time.Second))
	}
}

func TestInboxPolicyAllowlistAndBlocklist(t *testing.T) {
	app := NewApp(Config{
		Domain:          "team-a.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	handler := app.routes()

	pubAllowed, privAllowed, _ := ed25519.GenerateKey(crand.Reader)
	pubBlocked, privBlocked, _ := ed25519.GenerateKey(crand.Reader)
	pubTarget, privTarget, _ := ed25519.GenerateKey(crand.Reader)
	pubCross, privCross, _ := ed25519.GenerateKey(crand.Reader)

	registerUser := func(username, domain string, key ed25519.PublicKey, policy InboxPolicy) {
		t.Helper()
		body := mustJSON(t, registerRequest{
			Username:    username,
			Domain:      domain,
			PublicKey:   hex.EncodeToString(key),
			TTLSeconds:  3600,
			InboxPolicy: &policy,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("register %s@%s status = %d, body = %s", username, domain, resp.Code, resp.Body.String())
		}
	}

	registerUser("allowed", "team-a.test", pubAllowed, InboxPolicy{})
	registerUser("blocked", "team-a.test", pubBlocked, InboxPolicy{})
	registerUser("target", "team-a.test", pubTarget, InboxPolicy{})
	registerUser("partner", "team-b.test", pubCross, InboxPolicy{})

	send := func(fromEmail string, priv ed25519.PrivateKey, to string) int {
		t.Helper()
		body := mustJSON(t, sendRequest{To: to, Subject: "test", Body: "hi"})
		req := signedRequest(t, http.MethodPost, "/api/v1/send", body, fromEmail, priv)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		return resp.Code
	}

	if code := send("allowed@team-a.test", privAllowed, "target@team-a.test"); code != http.StatusOK {
		t.Fatalf("same-domain allowed sender status = %d, want 200", code)
	}
	if code := send("partner@team-b.test", privCross, "target@team-a.test"); code != http.StatusForbidden {
		t.Fatalf("cross-domain without allowlist status = %d, want 403", code)
	}

	policyBody := mustJSON(t, inboxPolicyResponse{
		InboxPolicy: InboxPolicy{
			Blocklist: []string{"blocked@team-a.test"},
			Allowlist: []string{"partner@team-b.test"},
		},
	})
	putReq := signedRequest(t, http.MethodPut, "/api/v1/account/inbox-policy", policyBody, "target@team-a.test", privTarget)
	putReq.Header.Set("Content-Type", "application/json")
	putResp := httptest.NewRecorder()
	handler.ServeHTTP(putResp, putReq)
	if putResp.Code != http.StatusOK {
		t.Fatalf("update inbox policy status = %d, body = %s", putResp.Code, putResp.Body.String())
	}

	if code := send("blocked@team-a.test", privBlocked, "target@team-a.test"); code != http.StatusForbidden {
		t.Fatalf("blocklisted same-domain sender status = %d, want 403", code)
	}
	if code := send("partner@team-b.test", privCross, "target@team-a.test"); code != http.StatusOK {
		t.Fatalf("allowlisted cross-domain sender status = %d, want 200", code)
	}

	getReq := signedRequest(t, http.MethodGet, "/api/v1/account/inbox-policy", nil, "target@team-a.test", privTarget)
	getResp := httptest.NewRecorder()
	handler.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("get inbox policy status = %d", getResp.Code)
	}
	var got inboxPolicyResponse
	if err := json.NewDecoder(getResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode inbox policy: %v", err)
	}
	if len(got.InboxPolicy.Blocklist) != 1 || len(got.InboxPolicy.Allowlist) != 1 {
		t.Fatalf("unexpected inbox policy: %+v", got.InboxPolicy)
	}
}

func TestSendRateLimitPerMailbox(t *testing.T) {
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	handler := app.routes()

	senderPub, senderPriv, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate sender key: %v", err)
	}
	recipientPub, _, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate recipient key: %v", err)
	}

	register := func(username string, key ed25519.PublicKey) {
		t.Helper()
		body := mustJSON(t, registerRequest{
			Username:   username,
			PublicKey:  hex.EncodeToString(key),
			TTLSeconds: 3600,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("register %s status = %d, body = %s", username, resp.Code, resp.Body.String())
		}
	}
	register("sender", senderPub)
	register("recipient", recipientPub)

	send := func() int {
		t.Helper()
		body := mustJSON(t, sendRequest{
			To:      "recipient@agent.test",
			Subject: "limited",
			Body:    "hello",
		})
		req := signedRequest(t, http.MethodPost, "/api/v1/send", body, "sender@agent.test", senderPriv)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		return resp.Code
	}

	for i := 0; i < 2; i++ {
		if code := send(); code != http.StatusOK {
			t.Fatalf("send %d status = %d, want 200", i+1, code)
		}
	}
	if code := send(); code != http.StatusTooManyRequests {
		t.Fatalf("third send status = %d, want 429", code)
	}
}

func TestFreeDomainRegistration(t *testing.T) {
	app := NewApp(Config{
		Domain:          "default.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	handler := app.routes()

	pubA, privA, _ := ed25519.GenerateKey(crand.Reader)
	pubB, _, _ := ed25519.GenerateKey(crand.Reader)
	pubPeer, privPeer, _ := ed25519.GenerateKey(crand.Reader)

	register := func(username, domain string, key ed25519.PublicKey) int {
		t.Helper()
		body := mustJSON(t, registerRequest{
			Username:   username,
			Domain:     domain,
			PublicKey:  hex.EncodeToString(key),
			TTLSeconds: 3600,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		return resp.Code
	}

	if code := register("bot", "custom-team.internal", pubA); code != http.StatusCreated {
		t.Fatalf("register custom domain status = %d, want 201", code)
	}
	if code := register("peer", "custom-team.internal", pubPeer); code != http.StatusCreated {
		t.Fatalf("register peer on custom domain status = %d, want 201", code)
	}
	if code := register("bot", "other-space.local", pubB); code != http.StatusCreated {
		t.Fatalf("register another custom domain status = %d, want 201", code)
	}
	if code := register("bot", "custom-team.internal", pubB); code != http.StatusConflict {
		t.Fatalf("duplicate mailbox status = %d, want 409", code)
	}

	sendBody := mustJSON(t, sendRequest{To: "peer@custom-team.internal", Subject: "hi", Body: "hello"})
	sendReq := signedRequest(t, http.MethodPost, "/api/v1/send", sendBody, "bot@custom-team.internal", privA)
	sendReq.Header.Set("Content-Type", "application/json")
	sendResp := httptest.NewRecorder()
	handler.ServeHTTP(sendResp, sendReq)
	if sendResp.Code != http.StatusOK {
		t.Fatalf("same custom-domain send status = %d, want 200", sendResp.Code)
	}

	sendBody = mustJSON(t, sendRequest{To: "bot@other-space.local", Subject: "hi", Body: "hello"})
	sendReq = signedRequest(t, http.MethodPost, "/api/v1/send", sendBody, "bot@custom-team.internal", privA)
	sendReq.Header.Set("Content-Type", "application/json")
	sendResp = httptest.NewRecorder()
	handler.ServeHTTP(sendResp, sendReq)
	if sendResp.Code != http.StatusForbidden {
		t.Fatalf("cross-domain send without allowlist status = %d, want 403", sendResp.Code)
	}

	sendBody = mustJSON(t, sendRequest{To: "nobody@unregistered.test", Subject: "hi", Body: "hello"})
	sendReq = signedRequest(t, http.MethodPost, "/api/v1/send", sendBody, "peer@custom-team.internal", privPeer)
	sendReq.Header.Set("Content-Type", "application/json")
	sendResp = httptest.NewRecorder()
	handler.ServeHTTP(sendResp, sendReq)
	if sendResp.Code != http.StatusNotFound {
		t.Fatalf("send to unregistered mailbox status = %d, want 404", sendResp.Code)
	}
}

func TestSendRejectsOversizeRequestBody(t *testing.T) {
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: 512,
	})
	handler := app.routes()

	publicKey, privateKey, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	registerBody := mustJSON(t, registerRequest{
		Username:   "bot_1",
		PublicKey:  hex.EncodeToString(publicKey),
		TTLSeconds: 3600,
	})
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerResp := httptest.NewRecorder()
	handler.ServeHTTP(registerResp, registerReq)
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", registerResp.Code, registerResp.Body.String())
	}

	sendBody := mustJSON(t, sendRequest{
		To:      "bot_1@agent.test",
		Subject: "too large",
		Body:    strings.Repeat("x", 600),
	})
	sendReq := signedRequest(t, http.MethodPost, "/api/v1/send", sendBody, "bot_1@agent.test", privateKey)
	sendReq.Header.Set("Content-Type", "application/json")
	sendResp := httptest.NewRecorder()
	handler.ServeHTTP(sendResp, sendReq)
	if sendResp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize send status = %d, want %d, body = %s", sendResp.Code, http.StatusRequestEntityTooLarge, sendResp.Body.String())
	}
}

func TestLoadConfigAppliesEnvOverridesAndDefaults(t *testing.T) {
	t.Setenv("AGENTPOST_DOMAIN", "Override.Example")
	t.Setenv("AGENTPOST_HTTP_ADDR", ":19090")
	t.Setenv("AGENTPOST_ALLOW_EXTERNAL_RELAY", "1")
	t.Setenv("AGENTPOST_API_TOKEN", "from-env")

	configPath := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(configPath, []byte(`
domain: File.Example
http_addr: ""
smtp_addr: ":2525"
allow_external_relay: false
max_message_bytes: 0
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Domain != "override.example" {
		t.Fatalf("domain = %q, want override.example", cfg.Domain)
	}
	if cfg.HTTPAddr != ":19090" {
		t.Fatalf("http addr = %q, want :19090", cfg.HTTPAddr)
	}
	if cfg.SMTPAddr != ":2525" {
		t.Fatalf("smtp addr = %q, want :2525", cfg.SMTPAddr)
	}
	if !cfg.AllowExternalRelay {
		t.Fatalf("allow external relay should be true from env override")
	}
	if cfg.APIToken != "from-env" {
		t.Fatalf("api token = %q, want from-env", cfg.APIToken)
	}
	if cfg.MaxMessageBytes != defaultMaxMessageBytes {
		t.Fatalf("max message bytes = %d, want %d", cfg.MaxMessageBytes, defaultMaxMessageBytes)
	}
}

func TestSkillEndpoint(t *testing.T) {
	t.Setenv("AGENTPOST_PUBLIC_URL", "https://gateway.example.com")
	t.Setenv("AGENTPOST_SCENARIO", "public-domain")

	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
		APIToken:        "secret-gateway-token",
	})
	handler := app.routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skill", nil)
	req.Host = "wrong.example.com"
	req.Header.Set("X-Forwarded-Proto", "http")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("skill status = %d, body = %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if strings.Contains(body, "secret-gateway-token") {
		t.Fatalf("skill must not contain the gateway token")
	}
	if !strings.Contains(body, "https://gateway.example.com") {
		t.Fatalf("skill should use AGENTPOST_PUBLIC_URL, got: %s", body)
	}
	if strings.Contains(body, "wrong.example.com") {
		t.Fatalf("skill must not fall back to request Host when PUBLIC_URL is set")
	}
	if !strings.Contains(body, "agent.test") {
		t.Fatalf("skill should include domain")
	}
	if !strings.Contains(body, "public-domain") {
		t.Fatalf("skill should include deployment scenario")
	}
	if !strings.Contains(body, "/api/v1/agents") {
		t.Fatalf("skill should document agent directory endpoint")
	}
	if !strings.Contains(body, "request / reply 对话协议") {
		t.Fatalf("skill should document request/reply protocol")
	}
	if !strings.Contains(body, "后台 subagent") {
		t.Fatalf("skill should document inbox subagent polling")
	}
	if !strings.Contains(body, "LLM Token 用量") {
		t.Fatalf("skill should document LLM token plan vs polling")
	}
	if !strings.Contains(body, "禁止空回复") {
		t.Fatalf("skill should require executing request before reply")
	}
	if !strings.Contains(body, "使用说明") {
		t.Fatalf("skill should be in Chinese by default")
	}

	jsonReq := httptest.NewRequest(http.MethodGet, "/api/v1/skill", nil)
	jsonReq.Header.Set("Accept", "application/json")
	jsonResp := httptest.NewRecorder()
	handler.ServeHTTP(jsonResp, jsonReq)
	if jsonResp.Code != http.StatusOK {
		t.Fatalf("skill json status = %d", jsonResp.Code)
	}
	var got skillResponse
	if err := json.NewDecoder(jsonResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode skill json: %v", err)
	}
	if got.Meta.Domain != "agent.test" || !got.Meta.GatewayToken {
		t.Fatalf("unexpected skill meta: %+v", got.Meta)
	}
	if got.Meta.ServerURL != "https://gateway.example.com" || got.Meta.PublicURLSource != "deployment_env" {
		t.Fatalf("unexpected skill URL meta: %+v", got.Meta)
	}
}

func TestSkillEndpointEnglish(t *testing.T) {
	t.Setenv("AGENTPOST_PUBLIC_URL", "https://gateway.example.com")
	t.Setenv("AGENTPOST_SCENARIO", "public-ip")

	app := NewApp(Config{
		Domain:          "example.domain",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
		APIToken:        "secret-gateway-token",
	})
	handler := app.routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skill?lang=en", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("skill status = %d, body = %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if resp.Header().Get("Content-Language") != "en" {
		t.Fatalf("Content-Language = %q, want en", resp.Header().Get("Content-Language"))
	}
	if strings.Contains(body, "secret-gateway-token") {
		t.Fatalf("skill must not contain the gateway token")
	}
	for _, want := range []string{
		"AgentPost Skill Guide",
		"https://gateway.example.com",
		"example.domain",
		"Request / reply conversation protocol",
		"Background inbox subagent",
		"LLM token plan usage",
		"empty acknowledgements are forbidden",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("English skill missing %q in body:\n%s", want, body)
		}
	}
	if strings.Contains(body, "使用说明") {
		t.Fatalf("English skill should not include the Chinese title")
	}

	jsonReq := httptest.NewRequest(http.MethodGet, "/api/v1/skill", nil)
	jsonReq.Header.Set("Accept", "application/json")
	jsonReq.Header.Set("Accept-Language", "en-US,en;q=0.9")
	jsonResp := httptest.NewRecorder()
	handler.ServeHTTP(jsonResp, jsonReq)
	if jsonResp.Code != http.StatusOK {
		t.Fatalf("skill json status = %d", jsonResp.Code)
	}
	if jsonResp.Header().Get("Content-Language") != "en" {
		t.Fatalf("json Content-Language = %q, want en", jsonResp.Header().Get("Content-Language"))
	}
	var got skillResponse
	if err := json.NewDecoder(jsonResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode skill json: %v", err)
	}
	if got.Meta.Language != "en" {
		t.Fatalf("unexpected skill language meta: %+v", got.Meta)
	}
	if !strings.Contains(got.Content, "AgentPost Skill Guide") {
		t.Fatalf("English json skill content missing title: %s", got.Content)
	}
}

func TestSkillEndpointInfersHostWhenPublicURLUnset(t *testing.T) {
	t.Setenv("AGENTPOST_PUBLIC_URL", "")

	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":8080",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	handler := app.routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skill", nil)
	req.Host = "gateway.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("skill status = %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "https://gateway.example.com") {
		t.Fatalf("skill should infer URL from request Host when PUBLIC_URL is unset")
	}
}

func TestAuthenticateRejectsStaleAndFutureTimestamps(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	app.now = func() time.Time { return now }
	handler := app.routes()

	publicKey, privateKey, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	registerBody := mustJSON(t, registerRequest{
		Username:   "bot_1",
		PublicKey:  hex.EncodeToString(publicKey),
		TTLSeconds: 3600,
	})
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerResp := httptest.NewRecorder()
	handler.ServeHTTP(registerResp, registerReq)
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", registerResp.Code, registerResp.Body.String())
	}

	for name, signedAt := range map[string]time.Time{
		"stale":  now.Add(-authTimestampTolerance - time.Second),
		"future": now.Add(authTimestampTolerance + time.Second),
	} {
		t.Run(name, func(t *testing.T) {
			req := signedRequestAt(t, http.MethodGet, "/api/v1/messages", nil, "bot_1@agent.test", privateKey, signedAt)
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("messages status = %d, want 401, body = %s", resp.Code, resp.Body.String())
			}
		})
	}
}

func TestRegisterRateLimitByClientIP(t *testing.T) {
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	handler := app.routes()

	publicKey, _, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	register := func(username string) int {
		t.Helper()
		body := mustJSON(t, registerRequest{
			Username:   username,
			PublicKey:  hex.EncodeToString(publicKey),
			TTLSeconds: 3600,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "198.51.100.50")
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		return resp.Code
	}

	for i := 0; i < registerRatePerMinute; i++ {
		if code := register("bot_" + strconv.Itoa(i)); code != http.StatusCreated {
			t.Fatalf("register %d status = %d, want %d", i, code, http.StatusCreated)
		}
	}
	if code := register("overflow"); code != http.StatusTooManyRequests {
		t.Fatalf("overflow register status = %d, want %d", code, http.StatusTooManyRequests)
	}
}

func TestGatewayTokenRequiredWhenConfigured(t *testing.T) {
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
		APIToken:        "secret-gateway-token",
	})
	handler := app.routes()

	publicKey, _, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	registerBody := mustJSON(t, registerRequest{
		Username:   "bot_1",
		PublicKey:  hex.EncodeToString(publicKey),
		TTLSeconds: 3600,
	})

	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerResp := httptest.NewRecorder()
	handler.ServeHTTP(registerResp, registerReq)
	if registerResp.Code != http.StatusUnauthorized {
		t.Fatalf("register without token status = %d, want %d", registerResp.Code, http.StatusUnauthorized)
	}

	registerReq = httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerReq.Header.Set("Authorization", "Bearer secret-gateway-token")
	registerResp = httptest.NewRecorder()
	handler.ServeHTTP(registerResp, registerReq)
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register with token status = %d, body = %s", registerResp.Code, registerResp.Body.String())
	}

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthResp := httptest.NewRecorder()
	handler.ServeHTTP(healthResp, healthReq)
	if healthResp.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want %d", healthResp.Code, http.StatusOK)
	}
}

func TestSendRejectsBadSignature(t *testing.T) {
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	handler := app.routes()

	publicKey, _, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	_, wrongPrivateKey, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate wrong key: %v", err)
	}

	registerBody := mustJSON(t, registerRequest{
		Username:   "bot_1",
		PublicKey:  hex.EncodeToString(publicKey),
		TTLSeconds: 3600,
	})
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerResp := httptest.NewRecorder()
	handler.ServeHTTP(registerResp, registerReq)
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", registerResp.Code, registerResp.Body.String())
	}

	sendBody := mustJSON(t, sendRequest{
		To:      "bot_1@agent.test",
		Subject: "hello",
		Body:    "this should not deliver",
	})
	sendReq := signedRequest(t, http.MethodPost, "/api/v1/send", sendBody, "bot_1@agent.test", wrongPrivateKey)
	sendReq.Header.Set("Content-Type", "application/json")
	sendResp := httptest.NewRecorder()
	handler.ServeHTTP(sendResp, sendReq)
	if sendResp.Code != http.StatusUnauthorized {
		t.Fatalf("send status = %d, want %d, body = %s", sendResp.Code, http.StatusUnauthorized, sendResp.Body.String())
	}
}

func TestSMTPHTMLIsConvertedToText(t *testing.T) {
	raw := []byte("From: human@example.com\r\n" +
		"To: bot_1@agent.test\r\n" +
		"Subject: Reset\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<html><body><p>Your code is <strong>889211</strong></p><script>ignore()</script></body></html>")

	parsed, err := parseMIMEMessage(raw)
	if err != nil {
		t.Fatalf("parse MIME: %v", err)
	}
	if parsed.BodyText == "" || parsed.BodyText == string(raw) {
		t.Fatalf("HTML was not converted to text: %q", parsed.BodyText)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return body
}

func signedRequest(t *testing.T, method, target string, body []byte, identity string, privateKey ed25519.PrivateKey) *http.Request {
	t.Helper()
	return signedRequestAt(t, method, target, body, identity, privateKey, time.Now())
}

func signedRequestAt(t *testing.T, method, target string, body []byte, identity string, privateKey ed25519.PrivateKey, signedAt time.Time) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	timestamp := strconv.FormatInt(signedAt.Unix(), 10)
	signature := ed25519.Sign(privateKey, signaturePayload(timestamp, body))
	if strings.Contains(identity, "@") {
		req.Header.Set("X-Agent-Email", identity)
	} else {
		req.Header.Set("X-Agent-Username", identity)
	}
	req.Header.Set("X-Agent-Timestamp", timestamp)
	req.Header.Set("X-Agent-Signature", hex.EncodeToString(signature))
	return req
}
