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
