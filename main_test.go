package main

import (
	"bytes"
	"crypto/ed25519"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	sendReq := signedRequest(t, http.MethodPost, "/api/v1/send", sendBody, "bot_1", privateKey)
	sendReq.Header.Set("Content-Type", "application/json")
	sendResp := httptest.NewRecorder()
	handler.ServeHTTP(sendResp, sendReq)
	if sendResp.Code != http.StatusOK {
		t.Fatalf("send status = %d, body = %s", sendResp.Code, sendResp.Body.String())
	}

	pollReq := signedRequest(t, http.MethodGet, "/api/v1/messages", nil, "bot_1", privateKey)
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

	listReq := signedRequest(t, http.MethodGet, "/api/v1/agents", nil, "bot_a", privA)
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
	sendReq := signedRequest(t, http.MethodPost, "/api/v1/send", sendBody, "bot_b", privB)
	sendReq.Header.Set("Content-Type", "application/json")
	sendResp := httptest.NewRecorder()
	handler.ServeHTTP(sendResp, sendReq)
	if sendResp.Code != http.StatusOK {
		t.Fatalf("send status = %d, body = %s", sendResp.Code, sendResp.Body.String())
	}

	delReq := signedRequest(t, http.MethodDelete, "/api/v1/account", nil, "bot_a", privA)
	delResp := httptest.NewRecorder()
	handler.ServeHTTP(delResp, delReq)
	if delResp.Code != http.StatusOK {
		t.Fatalf("unregister status = %d, body = %s", delResp.Code, delResp.Body.String())
	}

	listAfter := signedRequest(t, http.MethodGet, "/api/v1/agents", nil, "bot_b", privB)
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

	sendAfter := signedRequest(t, http.MethodPost, "/api/v1/send", sendBody, "bot_b", privB)
	sendAfter.Header.Set("Content-Type", "application/json")
	sendAfterResp := httptest.NewRecorder()
	handler.ServeHTTP(sendAfterResp, sendAfter)
	if sendAfterResp.Code != http.StatusNotFound {
		t.Fatalf("send to unregistered bot_a status = %d, want 404", sendAfterResp.Code)
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
	sendReq := signedRequest(t, http.MethodPost, "/api/v1/send", sendBody, "bot_1", wrongPrivateKey)
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

func signedRequest(t *testing.T, method, target string, body []byte, username string, privateKey ed25519.PrivateKey) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := ed25519.Sign(privateKey, signaturePayload(timestamp, body))
	req.Header.Set("X-Agent-Username", username)
	req.Header.Set("X-Agent-Timestamp", timestamp)
	req.Header.Set("X-Agent-Signature", hex.EncodeToString(signature))
	return req
}
