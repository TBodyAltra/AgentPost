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

	directedStatus := func(from, to string) string {
		for _, l := range got.Links {
			if l.From == from && l.To == to {
				return l.ForwardStatus
			}
			if l.From == to && l.To == from {
				return l.ReverseStatus
			}
		}
		return ""
	}

	if status := directedStatus("sender@team-a.test", "target@team-a.test"); status != "blocked" {
		t.Fatalf("sender -> target status = %q, want blocked", status)
	}
	if status := directedStatus("target@team-a.test", "sender@team-a.test"); status != "allowed" {
		t.Fatalf("target -> sender status = %q, want allowed", status)
	}
	if status := directedStatus("sender@team-a.test", "partner@team-b.test"); status != "allowlisted" {
		t.Fatalf("sender -> partner status = %q, want allowlisted", status)
	}
	if status := directedStatus("partner@team-b.test", "sender@team-a.test"); status != "cross_domain_blocked" {
		t.Fatalf("partner -> sender status = %q, want cross_domain_blocked", status)
	}
	if len(got.Links) != 3 {
		t.Fatalf("expected 3 mailbox pairs for 3 mailboxes, got %d", len(got.Links))
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

func TestDashboardAPIWorksWithoutTokenWhenDisabled(t *testing.T) {
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
		APIToken:        "",
	})
	handler := app.routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("dashboard without token status = %d, want 200, body = %s", resp.Code, resp.Body.String())
	}

	var got dashboardResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	if got.Gateway.GatewayToken {
		t.Fatalf("gateway_token_required = true, want false when API token disabled")
	}
}

func TestDashboardLinksHaveDirectedStatuses(t *testing.T) {
	app := NewApp(Config{
		Domain:          "team.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	handler := app.routes()

	pubA, _, _ := ed25519.GenerateKey(crand.Reader)
	pubB, _, _ := ed25519.GenerateKey(crand.Reader)
	registerDashboardUser(t, handler, "alice", "team.test", pubA, nil)
	registerDashboardUser(t, handler, "bob", "team.test", pubB, nil)

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
	if len(got.Links) != 1 {
		t.Fatalf("expected 1 link pair for 2 mailboxes, got %d", len(got.Links))
	}
	link := got.Links[0]
	if link.ForwardStatus == "" || link.ReverseStatus == "" {
		t.Fatalf("link missing directed statuses: %+v", link)
	}
	if link.ForwardStatus != "allowed" || link.ReverseStatus != "allowed" {
		t.Fatalf("same-domain pair statuses = forward %q reverse %q, want allowed/allowed", link.ForwardStatus, link.ReverseStatus)
	}
}

func registerDashboardUser(t *testing.T, handler http.Handler, username, domain string, key ed25519.PublicKey, policy *InboxPolicy) {
	registerDashboardUserWithGateway(t, handler, "", username, domain, key, policy)
}

func registerDashboardUserWithGateway(t *testing.T, handler http.Handler, gatewayToken, username, domain string, key ed25519.PublicKey, policy *InboxPolicy) {
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
	if gatewayToken != "" {
		req.Header.Set("Authorization", "Bearer "+gatewayToken)
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("register %s@%s status = %d, body = %s", username, domain, resp.Code, resp.Body.String())
	}
}

func TestDashboardDirectedDeliveryStatus(t *testing.T) {
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

	if status := dashboardDirectedStatus("alice@a.test", "bob@a.test", users["bob@a.test"].InboxPolicy); status != "blocked" {
		t.Fatalf("alice -> bob = %q, want blocked", status)
	}
	if status := dashboardDirectedStatus("bob@a.test", "alice@a.test", users["alice@a.test"].InboxPolicy); status != "allowed" {
		t.Fatalf("bob -> alice = %q, want allowed", status)
	}
	if status := dashboardDirectedStatus("alice@a.test", "carol@b.test", users["carol@b.test"].InboxPolicy); status != "allowlisted" {
		t.Fatalf("alice -> carol = %q, want allowlisted", status)
	}
	if status := dashboardDirectedStatus("carol@b.test", "alice@a.test", users["alice@a.test"].InboxPolicy); status != "cross_domain_blocked" {
		t.Fatalf("carol -> alice = %q, want cross_domain_blocked", status)
	}

	users["dave@a.test"] = &User{Username: "dave", Domain: "a.test"}
	if status := dashboardDirectedStatus("alice@a.test", "dave@a.test", users["dave@a.test"].InboxPolicy); status != "allowed" {
		t.Fatalf("same-domain open delivery = %q, want allowed", status)
	}
}
