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

	pairStatus := func(a, b string) string {
		if a > b {
			a, b = b, a
		}
		for _, l := range got.Links {
			if l.From == a && l.To == b {
				return l.Status
			}
		}
		return ""
	}

	if status := pairStatus("sender@team-a.test", "target@team-a.test"); status != "blocked" {
		t.Fatalf("blocklist pair status = %q, want blocked (one-way block breaks bidirectional link)", status)
	}
	if status := pairStatus("sender@team-a.test", "partner@team-b.test"); status != "cross_domain_blocked" {
		t.Fatalf("asymmetric allowlist pair status = %q, want cross_domain_blocked", status)
	}
	if len(got.Links) != 3 {
		t.Fatalf("expected 3 undirected links for 3 mailboxes, got %d", len(got.Links))
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

func TestDashboardPairStatusBidirectional(t *testing.T) {
	users := map[string]*User{
		"alice@a.test": {
			Username: "alice", Domain: "a.test",
			InboxPolicy: InboxPolicy{},
		},
		"bob@a.test": {
			Username: "bob", Domain: "a.test",
			InboxPolicy: InboxPolicy{Blocklist: []string{"alice@a.test"}},
		},
		"carol@b.test": {
			Username: "carol", Domain: "b.test",
			InboxPolicy: InboxPolicy{Allowlist: []string{"alice@a.test"}},
		},
	}

	if status := dashboardPairStatus("alice@a.test", "bob@a.test", users); status != "blocked" {
		t.Fatalf("one-way blocklist pair = %q, want blocked", status)
	}
	if status := dashboardPairStatus("alice@a.test", "carol@b.test", users); status != "cross_domain_blocked" {
		t.Fatalf("one-way allowlist pair = %q, want cross_domain_blocked", status)
	}

	users["dave@a.test"] = &User{Username: "dave", Domain: "a.test"}
	if status := dashboardPairStatus("alice@a.test", "dave@a.test", users); status != "allowed" {
		t.Fatalf("same-domain open pair = %q, want allowed", status)
	}
}
