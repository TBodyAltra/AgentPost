package main

import (
	"bytes"
	"crypto/ed25519"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDashboardAPIRequiresGatewayTokenWhenConfigured(t *testing.T) {
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
		APIToken:        "secret-gateway-token",
	})
	handler := app.routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("dashboard without token status = %d, want 401", resp.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	req.Header.Set("Authorization", "Bearer secret-gateway-token")
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("dashboard with token status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

func TestDashboardUIIsPublic(t *testing.T) {
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
		APIToken:        "secret-gateway-token",
	})
	handler := app.routes()

	req := httptest.NewRequest(http.MethodGet, "/dashboard/", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("dashboard UI status = %d, want 200", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); ct == "" {
		t.Fatalf("dashboard UI should set content type")
	}
}

func TestDashboardSnapshotDomainsAndLinks(t *testing.T) {
	app := NewApp(Config{
		Domain:          "team-a.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	handler := app.routes()

	pubA, _, _ := ed25519.GenerateKey(crand.Reader)
	pubB, _, _ := ed25519.GenerateKey(crand.Reader)
	pubPartner, _, _ := ed25519.GenerateKey(crand.Reader)

	register := func(username, domain string, key ed25519.PublicKey, policy *InboxPolicy) {
		t.Helper()
		body := mustJSON(t, registerRequest{
			Username:    username,
			Domain:      domain,
			PublicKey:   hex.EncodeToString(key),
			TTLSeconds:  3600,
			InboxPolicy: policy,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("register %s@%s status = %d, body = %s", username, domain, resp.Code, resp.Body.String())
		}
	}

	register("sender", "team-a.test", pubA, nil)
	register("target", "team-a.test", pubB, &InboxPolicy{Blocklist: []string{"sender@team-a.test"}})
	register("partner", "team-b.test", pubPartner, &InboxPolicy{Allowlist: []string{"sender@team-a.test"}})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, body = %s", resp.Code, resp.Body.String())
	}

	var got dashboardResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	if got.Gateway.ActiveMailboxes != 3 || got.Gateway.DomainCount != 2 {
		t.Fatalf("unexpected gateway stats: %+v", got.Gateway)
	}
	if len(got.Domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(got.Domains))
	}

	linkStatus := func(from, to string) string {
		for _, l := range got.Links {
			if l.From == from && l.To == to {
				return l.Status
			}
		}
		return ""
	}

	if status := linkStatus("sender@team-a.test", "target@team-a.test"); status != "blocked" {
		t.Fatalf("same-domain blocklist status = %q, want blocked", status)
	}
	if status := linkStatus("sender@team-a.test", "partner@team-b.test"); status != "allowlisted" {
		t.Fatalf("cross-domain allowlist status = %q, want allowlisted", status)
	}
	if status := linkStatus("partner@team-b.test", "sender@team-a.test"); status != "cross_domain_blocked" {
		t.Fatalf("default cross-domain status = %q, want cross_domain_blocked", status)
	}

	var target *dashboardMailbox
	for i := range got.Mailboxes {
		if got.Mailboxes[i].Email == "target@team-a.test" {
			target = &got.Mailboxes[i]
			break
		}
	}
	if target == nil || len(target.InboxPolicy.Blocklist) != 1 {
		t.Fatalf("target mailbox detail missing blocklist: %+v", target)
	}
}
