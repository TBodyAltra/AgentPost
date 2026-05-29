package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"net/mail"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	smtp "github.com/emersion/go-smtp"
	"github.com/jaytaylor/html2text"
	"golang.org/x/time/rate"
	"gopkg.in/yaml.v3"
)

const (
	defaultDomain          = "localhost"
	defaultHTTPAddr        = ":8080"
	defaultSMTPAddr        = ":2525"
	defaultTTLSeconds      = 3600
	maxTTLSeconds          = 86400
	authTimestampTolerance = 5 * time.Minute
	defaultMaxMessageBytes = 1 << 20
	maxProfileFieldLen     = 512
	maxProfileNotesLen     = 2048
	maxProfileListItems    = 32
	maxProfileListItemLen  = 128
	maxInboxPolicyItems    = 64
)

type Config struct {
	Domain             string `yaml:"domain"`
	HTTPAddr           string `yaml:"http_addr"`
	SMTPAddr           string `yaml:"smtp_addr"`
	AllowExternalRelay bool   `yaml:"allow_external_relay"`
	MaxMessageBytes    int64  `yaml:"max_message_bytes"`
	APIToken           string `yaml:"-"`
}

type App struct {
	cfg Config
	now func() time.Time

	mu       sync.RWMutex
	users    map[string]*User
	messages map[string][]Message
	limiters map[string]*rate.Limiter
}

type AgentProfile struct {
	DisplayName      string   `json:"display_name,omitempty"`
	Host             string   `json:"host,omitempty"`
	Responsibilities string   `json:"responsibilities,omitempty"`
	Skills           []string `json:"skills,omitempty"`
	MCPServices      []string `json:"mcp_services,omitempty"`
	Capabilities     []string `json:"capabilities,omitempty"`
	Notes            string   `json:"notes,omitempty"`
}

type InboxPolicy struct {
	Blocklist []string `json:"blocklist,omitempty"`
	Allowlist []string `json:"allowlist,omitempty"`
}

type User struct {
	Username     string
	Domain       string
	PublicKey    ed25519.PublicKey
	ExpiresAt    time.Time
	RegisteredAt time.Time
	Profile      AgentProfile
	InboxPolicy  InboxPolicy
}

type Message struct {
	MessageID  string    `json:"message_id"`
	From       string    `json:"from"`
	To         string    `json:"to,omitempty"`
	Subject    string    `json:"subject"`
	BodyText   string    `json:"body_text"`
	ReceivedAt time.Time `json:"received_at"`
}

type registerRequest struct {
	Username    string        `json:"username"`
	Domain      string        `json:"domain,omitempty"`
	PublicKey   string        `json:"public_key"`
	TTLSeconds  int64         `json:"ttl_seconds"`
	Profile     *AgentProfile `json:"profile,omitempty"`
	InboxPolicy *InboxPolicy  `json:"inbox_policy,omitempty"`
}

type registerResponse struct {
	Email        string       `json:"email"`
	ExpiresAt    time.Time    `json:"expires_at"`
	RegisteredAt time.Time    `json:"registered_at"`
	Profile      AgentProfile `json:"profile,omitempty"`
	InboxPolicy  InboxPolicy  `json:"inbox_policy"`
	Status       string       `json:"status"`
}

type agentEntry struct {
	Username     string       `json:"username"`
	Domain       string       `json:"domain"`
	Email        string       `json:"email"`
	ExpiresAt    time.Time    `json:"expires_at"`
	RegisteredAt time.Time    `json:"registered_at"`
	Profile      AgentProfile `json:"profile,omitempty"`
}

type agentsResponse struct {
	Agents []agentEntry `json:"agents"`
}

type unregisterResponse struct {
	Email  string `json:"email"`
	Status string `json:"status"`
}

type inboxPolicyResponse struct {
	InboxPolicy InboxPolicy `json:"inbox_policy"`
}

type sendRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type sendResponse struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

type deliverResult int

const (
	deliverOK deliverResult = iota
	deliverRecipientMissing
	deliverRejectedByPolicy
)

type messagesResponse struct {
	Messages []Message `json:"messages"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func main() {
	configPath := flag.String("config", envOrDefault("AGENTPOST_CONFIG", "config.yaml"), "path to config.yaml")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	app := NewApp(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app.StartJanitor(ctx)

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("AgentPost HTTP listening on %s (default domain %s)", cfg.HTTPAddr, cfg.Domain)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	if cfg.SMTPAddr != "" {
		smtpServer := newSMTPServer(app)
		go func() {
			log.Printf("AgentPost SMTP listening on %s", cfg.SMTPAddr)
			if err := smtpServer.ListenAndServe(); err != nil {
				errCh <- err
			}
		}()
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = smtpServer.Shutdown(shutdownCtx)
		}()
	}

	select {
	case <-ctx.Done():
	case err := <-errCh:
		log.Fatalf("server error: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}

func defaultConfig() Config {
	return Config{
		Domain:          defaultDomain,
		HTTPAddr:        defaultHTTPAddr,
		SMTPAddr:        defaultSMTPAddr,
		MaxMessageBytes: defaultMaxMessageBytes,
	}
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			applyEnvOverrides(&cfg)
			return cfg, nil
		}
		return cfg, err
	}
	defer file.Close()

	if err := yaml.NewDecoder(file).Decode(&cfg); err != nil {
		return cfg, err
	}
	if cfg.Domain == "" {
		cfg.Domain = defaultDomain
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = defaultHTTPAddr
	}
	if cfg.MaxMessageBytes <= 0 {
		cfg.MaxMessageBytes = defaultMaxMessageBytes
	}
	normalizeConfigDomains(&cfg)
	applyEnvOverrides(&cfg)
	normalizeConfigDomains(&cfg)
	return cfg, nil
}

func normalizeConfigDomains(cfg *Config) {
	cfg.Domain = strings.ToLower(strings.TrimSpace(cfg.Domain))
	if cfg.Domain == "" {
		cfg.Domain = defaultDomain
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("AGENTPOST_DOMAIN"); v != "" {
		cfg.Domain = v
	}
	if v := os.Getenv("AGENTPOST_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if v := os.Getenv("AGENTPOST_SMTP_ADDR"); v != "" {
		cfg.SMTPAddr = v
	}
	if v := os.Getenv("AGENTPOST_ALLOW_EXTERNAL_RELAY"); v != "" {
		cfg.AllowExternalRelay = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("AGENTPOST_API_TOKEN"); v != "" {
		cfg.APIToken = v
	}
}

func NewApp(cfg Config) *App {
	normalizeConfigDomains(&cfg)
	if cfg.MaxMessageBytes <= 0 {
		cfg.MaxMessageBytes = defaultMaxMessageBytes
	}
	return &App{
		cfg:      cfg,
		now:      time.Now,
		users:    make(map[string]*User),
		messages: make(map[string][]Message),
		limiters: make(map[string]*rate.Limiter),
	}
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/register", a.handleRegister)
	mux.HandleFunc("/api/v1/account", a.handleAccount)
	mux.HandleFunc("/api/v1/account/inbox-policy", a.handleInboxPolicy)
	mux.HandleFunc("/api/v1/agents", a.handleAgents)
	mux.HandleFunc("/api/v1/send", a.handleSend)
	mux.HandleFunc("/api/v1/messages", a.handleMessages)
	mux.HandleFunc("/api/v1/skill", a.handleSkill)
	mux.HandleFunc("/api/v1/dashboard", a.handleDashboardAPI)
	mux.Handle("/dashboard/", a.dashboardHandler())
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusFound)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return a.withGatewayAuth(mux)
}

func (a *App) withGatewayAuth(next http.Handler) http.Handler {
	token := strings.TrimSpace(a.cfg.APIToken)
	if token == "" {
		return next
	}
	expected := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/api/v1/skill" {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		got := extractGatewayToken(r)
		if got == "" || subtle.ConstantTimeCompare([]byte(got), expected) != 1 {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing or invalid API token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractGatewayToken(r *http.Request) string {
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); auth != "" {
		const prefix = "Bearer "
		if strings.HasPrefix(auth, prefix) {
			return strings.TrimSpace(auth[len(prefix):])
		}
	}
	if v := strings.TrimSpace(r.Header.Get("X-AgentPost-Token")); v != "" {
		return v
	}
	return ""
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if !hasJSONContentType(r) {
		writeJSON(w, http.StatusUnsupportedMediaType, errorResponse{Error: "Content-Type must be application/json"})
		return
	}

	var req registerRequest
	if err := decodeJSONBody(r, a.cfg.MaxMessageBytes, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	username := strings.ToLower(strings.TrimSpace(req.Username))
	if !validUsername(username) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "username must be 1-64 chars: lowercase letters, numbers, underscore, or hyphen"})
		return
	}

	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	if domain == "" {
		domain = a.cfg.Domain
	}
	if !validMailboxDomain(domain) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "domain must be a valid mailbox suffix"})
		return
	}
	mailbox := mailboxKey(username, domain)

	publicKey, err := hex.DecodeString(strings.TrimSpace(req.PublicKey))
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "public_key must be a hex-encoded Ed25519 public key"})
		return
	}

	ttl := req.TTLSeconds
	if ttl <= 0 {
		ttl = defaultTTLSeconds
	}
	if ttl > maxTTLSeconds {
		ttl = maxTTLSeconds
	}

	profile := AgentProfile{}
	if req.Profile != nil {
		normalized, err := normalizeAgentProfile(*req.Profile)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		profile = normalized
	}

	inboxPolicy := defaultInboxPolicy()
	if req.InboxPolicy != nil {
		normalized, err := a.normalizeInboxPolicy(*req.InboxPolicy)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		inboxPolicy = normalized
	}

	now := a.now().UTC()
	expiresAt := now.Add(time.Duration(ttl) * time.Second)

	a.mu.Lock()
	if existing, ok := a.users[mailbox]; ok && existing.ExpiresAt.After(now) {
		a.mu.Unlock()
		writeJSON(w, http.StatusConflict, errorResponse{Error: "mailbox is already registered"})
		return
	}
	a.users[mailbox] = &User{
		Username:     username,
		Domain:       domain,
		PublicKey:    append(ed25519.PublicKey(nil), publicKey...),
		ExpiresAt:    expiresAt,
		RegisteredAt: now,
		Profile:      profile,
		InboxPolicy:  inboxPolicy,
	}
	a.messages[mailbox] = nil
	a.limiters[mailbox] = rate.NewLimiter(rate.Every(time.Minute/2), 2)
	a.mu.Unlock()

	writeJSON(w, http.StatusCreated, registerResponse{
		Email:        mailbox,
		ExpiresAt:    expiresAt,
		RegisteredAt: now,
		Profile:      profile,
		InboxPolicy:  inboxPolicy,
		Status:       "active",
	})
}

func (a *App) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}

	if _, status, err := a.authenticate(r, nil); err != nil {
		writeJSON(w, status, errorResponse{Error: err.Error()})
		return
	}

	now := a.now().UTC()
	a.mu.RLock()
	entries := make([]agentEntry, 0, len(a.users))
	for _, user := range a.users {
		if !user.ExpiresAt.After(now) {
			continue
		}
		entries = append(entries, agentEntry{
			Username:     user.Username,
			Domain:       user.Domain,
			Email:        userMailbox(user),
			ExpiresAt:    user.ExpiresAt,
			RegisteredAt: user.RegisteredAt,
			Profile:      user.Profile,
		})
	}
	a.mu.RUnlock()

	writeJSON(w, http.StatusOK, agentsResponse{Agents: entries})
}

func (a *App) handleAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}

	user, status, err := a.authenticate(r, nil)
	if err != nil {
		writeJSON(w, status, errorResponse{Error: err.Error()})
		return
	}

	email := userMailbox(user)
	a.mu.Lock()
	a.deleteUserLocked(email)
	a.mu.Unlock()

	writeJSON(w, http.StatusOK, unregisterResponse{
		Email:  email,
		Status: "unregistered",
	})
}

func (a *App) handleInboxPolicy(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		user, status, err := a.authenticate(r, nil)
		if err != nil {
			writeJSON(w, status, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, inboxPolicyResponse{InboxPolicy: user.InboxPolicy})
	case http.MethodPut:
		if !hasJSONContentType(r) {
			writeJSON(w, http.StatusUnsupportedMediaType, errorResponse{Error: "Content-Type must be application/json"})
			return
		}
		body, err := readLimited(r.Body, a.cfg.MaxMessageBytes)
		if err != nil {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: err.Error()})
			return
		}
		user, status, err := a.authenticate(r, body)
		if err != nil {
			writeJSON(w, status, errorResponse{Error: err.Error()})
			return
		}
		var req inboxPolicyResponse
		if err := decodeJSON(bytes.NewReader(body), &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		policy, err := a.normalizeInboxPolicy(req.InboxPolicy)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		a.mu.Lock()
		if stored, ok := a.users[userMailbox(user)]; ok {
			stored.InboxPolicy = policy
		}
		a.mu.Unlock()
		writeJSON(w, http.StatusOK, inboxPolicyResponse{InboxPolicy: policy})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
	}
}

func (a *App) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if !hasJSONContentType(r) {
		writeJSON(w, http.StatusUnsupportedMediaType, errorResponse{Error: "Content-Type must be application/json"})
		return
	}

	body, err := readLimited(r.Body, a.cfg.MaxMessageBytes)
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: err.Error()})
		return
	}

	user, status, err := a.authenticate(r, body)
	if err != nil {
		writeJSON(w, status, errorResponse{Error: err.Error()})
		return
	}

	a.mu.RLock()
	limiter := a.limiters[userMailbox(user)]
	allowed := limiter != nil && limiter.Allow()
	a.mu.RUnlock()
	if !allowed {
		writeJSON(w, http.StatusTooManyRequests, errorResponse{Error: "send rate limit exceeded"})
		return
	}

	var req sendRequest
	if err := decodeJSON(bytes.NewReader(body), &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	to, err := normalizeEmail(req.To)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid recipient email"})
		return
	}

	username, domain, ok := splitEmail(to)
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid recipient email"})
		return
	}

	if !a.mailboxRegistered(username, domain) {
		if !a.cfg.AllowExternalRelay {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "recipient is not registered or has expired"})
			return
		}
		writeJSON(w, http.StatusNotImplemented, errorResponse{Error: "external relay is not implemented in the MVP"})
		return
	}

	message := Message{
		MessageID:  newMessageID(),
		From:       userMailbox(user),
		To:         to,
		Subject:    strings.TrimSpace(req.Subject),
		BodyText:   strings.TrimSpace(req.Body),
		ReceivedAt: a.now().UTC(),
	}

	switch a.deliver(mailboxKey(username, domain), message) {
	case deliverRecipientMissing:
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "recipient is not registered or has expired"})
		return
	case deliverRejectedByPolicy:
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "recipient inbox policy rejected this sender"})
		return
	}

	writeJSON(w, http.StatusOK, sendResponse{
		MessageID: message.MessageID,
		Status:    "delivered",
	})
}

func (a *App) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}

	user, status, err := a.authenticate(r, nil)
	if err != nil {
		writeJSON(w, status, errorResponse{Error: err.Error()})
		return
	}

	a.mu.Lock()
	mailbox := userMailbox(user)
	messages := append([]Message(nil), a.messages[mailbox]...)
	a.messages[mailbox] = nil
	a.mu.Unlock()

	writeJSON(w, http.StatusOK, messagesResponse{Messages: messages})
}

func (a *App) authenticate(r *http.Request, body []byte) (*User, int, error) {
	mailbox, err := a.parseAgentMailbox(agentIdentityFromRequest(r))
	if err != nil {
		return nil, http.StatusUnauthorized, err
	}

	timestamp := strings.TrimSpace(r.Header.Get("X-Agent-Timestamp"))
	if timestamp == "" {
		return nil, http.StatusUnauthorized, errors.New("missing X-Agent-Timestamp")
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return nil, http.StatusUnauthorized, errors.New("invalid X-Agent-Timestamp")
	}
	signedAt := time.Unix(ts, 0)
	if d := a.now().Sub(signedAt); d > authTimestampTolerance || d < -authTimestampTolerance {
		return nil, http.StatusUnauthorized, errors.New("X-Agent-Timestamp is outside the allowed window")
	}

	signature, err := hex.DecodeString(strings.TrimSpace(r.Header.Get("X-Agent-Signature")))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return nil, http.StatusUnauthorized, errors.New("X-Agent-Signature must be a hex-encoded Ed25519 signature")
	}

	a.mu.RLock()
	user, ok := a.users[mailbox]
	if !ok || !user.ExpiresAt.After(a.now()) {
		a.mu.RUnlock()
		return nil, http.StatusUnauthorized, errors.New("account is not registered or has expired")
	}
	userCopy := *user
	userCopy.PublicKey = append(ed25519.PublicKey(nil), user.PublicKey...)
	a.mu.RUnlock()

	if !ed25519.Verify(userCopy.PublicKey, signaturePayload(timestamp, body), signature) {
		return nil, http.StatusUnauthorized, errors.New("signature verification failed")
	}
	return &userCopy, http.StatusOK, nil
}

func signaturePayload(timestamp string, body []byte) []byte {
	payload := make([]byte, 0, len(timestamp)+1+len(body))
	payload = append(payload, timestamp...)
	payload = append(payload, '\n')
	payload = append(payload, body...)
	return payload
}

func (a *App) deliver(mailbox string, message Message) deliverResult {
	mailbox = strings.ToLower(strings.TrimSpace(mailbox))
	now := a.now()

	a.mu.Lock()
	defer a.mu.Unlock()
	user, ok := a.users[mailbox]
	if !ok || !user.ExpiresAt.After(now) {
		return deliverRecipientMissing
	}
	if !senderAllowedByInboxPolicy(user.InboxPolicy, message.From, user.Domain) {
		return deliverRejectedByPolicy
	}
	a.messages[mailbox] = append(a.messages[mailbox], message)
	return deliverOK
}

func (a *App) StartJanitor(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.cleanupExpired()
			}
		}
	}()
}

func (a *App) cleanupExpired() {
	now := a.now()

	a.mu.Lock()
	defer a.mu.Unlock()
	for mailbox, user := range a.users {
		if !user.ExpiresAt.After(now) {
			a.deleteUserLocked(mailbox)
		}
	}
}

func (a *App) deleteUserLocked(mailbox string) {
	delete(a.users, mailbox)
	delete(a.messages, mailbox)
	delete(a.limiters, mailbox)
}

func normalizeAgentProfile(profile AgentProfile) (AgentProfile, error) {
	profile.DisplayName = trimProfileField(profile.DisplayName, maxProfileFieldLen)
	profile.Host = trimProfileField(profile.Host, maxProfileFieldLen)
	profile.Responsibilities = trimProfileField(profile.Responsibilities, maxProfileFieldLen)
	profile.Notes = trimProfileField(profile.Notes, maxProfileNotesLen)

	skills, err := normalizeProfileList(profile.Skills, "profile.skills")
	if err != nil {
		return AgentProfile{}, err
	}
	mcpServices, err := normalizeProfileList(profile.MCPServices, "profile.mcp_services")
	if err != nil {
		return AgentProfile{}, err
	}
	capabilities, err := normalizeProfileList(profile.Capabilities, "profile.capabilities")
	if err != nil {
		return AgentProfile{}, err
	}

	profile.Skills = skills
	profile.MCPServices = mcpServices
	profile.Capabilities = capabilities
	return profile, nil
}

func trimProfileField(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if len(value) > maxLen {
		value = value[:maxLen]
	}
	return value
}

func normalizeProfileList(items []string, field string) ([]string, error) {
	if len(items) > maxProfileListItems {
		return nil, fmt.Errorf("%s must contain at most %d items", field, maxProfileListItems)
	}
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = trimProfileField(item, maxProfileListItemLen)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out, nil
}

func defaultInboxPolicy() InboxPolicy {
	return InboxPolicy{}
}

func (a *App) normalizeInboxPolicy(policy InboxPolicy) (InboxPolicy, error) {
	blocklist, err := a.normalizePolicyAddressList(policy.Blocklist, "inbox_policy.blocklist")
	if err != nil {
		return InboxPolicy{}, err
	}
	allowlist, err := a.normalizePolicyAddressList(policy.Allowlist, "inbox_policy.allowlist")
	if err != nil {
		return InboxPolicy{}, err
	}
	return InboxPolicy{Blocklist: blocklist, Allowlist: allowlist}, nil
}

func (a *App) normalizePolicyAddressList(items []string, field string) ([]string, error) {
	if len(items) > maxInboxPolicyItems {
		return nil, fmt.Errorf("%s must contain at most %d items", field, maxInboxPolicyItems)
	}
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		normalized, err := a.normalizePolicyAddress(item)
		if err != nil {
			return nil, err
		}
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func (a *App) normalizePolicyAddress(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.Contains(value, "@") {
		email, err := normalizeEmail(value)
		if err != nil {
			return "", fmt.Errorf("invalid inbox policy address %q", value)
		}
		username, domain, ok := splitEmail(email)
		if !ok || !validUsername(username) || !validMailboxDomain(domain) {
			return "", fmt.Errorf("invalid inbox policy address %q", value)
		}
		return mailboxKey(username, domain), nil
	}
	if !validUsername(value) {
		return "", fmt.Errorf("invalid inbox policy address %q", value)
	}
	return mailboxKey(strings.ToLower(value), a.cfg.Domain), nil
}

func senderAllowedByInboxPolicy(policy InboxPolicy, from string, recipientDomain string) bool {
	from = strings.ToLower(strings.TrimSpace(from))
	if from == "" {
		return false
	}
	if addressListMatches(from, policy.Blocklist, recipientDomain) {
		return false
	}

	_, fromDomain, ok := splitEmail(from)
	if !ok {
		return false
	}
	if fromDomain == recipientDomain {
		return true
	}
	return addressListMatches(from, policy.Allowlist, recipientDomain)
}

func addressListMatches(from string, addresses []string, recipientDomain string) bool {
	fromUser, fromDomain, fromOK := splitEmail(from)
	for _, addr := range addresses {
		if strings.EqualFold(from, addr) {
			return true
		}
		listUser, listDomain, listOK := splitEmail(addr)
		if !listOK {
			continue
		}
		if fromOK && fromUser == listUser && fromDomain == listDomain {
			return true
		}
		if fromOK && fromUser == listUser && listDomain == recipientDomain {
			return true
		}
	}
	return false
}

func (a *App) mailboxRegistered(username, domain string) bool {
	now := a.now()
	a.mu.RLock()
	defer a.mu.RUnlock()
	user, ok := a.users[mailboxKey(username, domain)]
	return ok && user.ExpiresAt.After(now)
}

func (a *App) parseAgentMailbox(identity string) (string, error) {
	identity = strings.ToLower(strings.TrimSpace(identity))
	if identity == "" {
		return "", errors.New("missing X-Agent-Email or X-Agent-Username")
	}
	if strings.Contains(identity, "@") {
		email, err := normalizeEmail(identity)
		if err != nil {
			return "", errors.New("invalid agent mailbox identity")
		}
		username, domain, ok := splitEmail(email)
		if !ok || !validUsername(username) || !validMailboxDomain(domain) {
			return "", errors.New("invalid agent mailbox identity")
		}
		return mailboxKey(username, domain), nil
	}
	if !validUsername(identity) {
		return "", errors.New("invalid X-Agent-Username; use full email user@domain on multi-domain gateways")
	}
	return mailboxKey(identity, a.cfg.Domain), nil
}

func agentIdentityFromRequest(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Agent-Email")); v != "" {
		return v
	}
	return strings.TrimSpace(r.Header.Get("X-Agent-Username"))
}

func mailboxKey(username, domain string) string {
	return strings.ToLower(username) + "@" + strings.ToLower(domain)
}

func userMailbox(user *User) string {
	return mailboxKey(user.Username, user.Domain)
}

func validMailboxDomain(domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" || len(domain) > 253 || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if !validDomainLabel(label) {
			return false
		}
	}
	return true
}

func validDomainLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 {
		return false
	}
	if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
		return false
	}
	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

func hasJSONContentType(r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "application/json"
}

func decodeJSONBody(r *http.Request, maxBytes int64, dst any) error {
	body, err := readLimited(r.Body, maxBytes)
	if err != nil {
		return err
	}
	return decodeJSON(bytes.NewReader(body), dst)
}

func decodeJSON(r io.Reader, dst any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("invalid JSON: multiple JSON values")
	}
	return nil
}

func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxMessageBytes
	}
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("request body exceeds %d bytes", maxBytes)
	}
	return body, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}

func validUsername(username string) bool {
	if len(username) == 0 || len(username) > 64 {
		return false
	}
	for _, r := range username {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func normalizeEmail(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("empty email")
	}
	if addr, err := mail.ParseAddress(value); err == nil {
		return strings.ToLower(addr.Address), nil
	}
	if strings.Count(value, "@") != 1 || strings.ContainsAny(value, " \t\r\n<>") {
		return "", errors.New("invalid email")
	}
	return strings.ToLower(value), nil
}

func splitEmail(email string) (username string, domain string, ok bool) {
	local, domain, found := strings.Cut(email, "@")
	if !found || local == "" || domain == "" {
		return "", "", false
	}
	return strings.ToLower(local), strings.ToLower(domain), true
}

func newMessageID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	return "msg_" + hex.EncodeToString(b[:])
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func newSMTPServer(app *App) *smtp.Server {
	server := smtp.NewServer(&smtpBackend{app: app})
	server.Addr = app.cfg.SMTPAddr
	server.Domain = app.cfg.Domain
	server.ReadTimeout = 10 * time.Second
	server.WriteTimeout = 10 * time.Second
	server.MaxMessageBytes = app.cfg.MaxMessageBytes
	server.MaxRecipients = 50
	return server
}

type smtpBackend struct {
	app *App
}

func (b *smtpBackend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &smtpSession{app: b.app}, nil
}

type smtpSession struct {
	app   *App
	from  string
	rcpts []string
}

func (s *smtpSession) Reset() {
	s.from = ""
	s.rcpts = nil
}

func (s *smtpSession) Logout() error {
	return nil
}

func (s *smtpSession) Mail(from string, _ *smtp.MailOptions) error {
	normalized, err := normalizeEmail(from)
	if err != nil {
		return &smtp.SMTPError{Code: 553, Message: "invalid sender address"}
	}
	s.from = normalized
	return nil
}

func (s *smtpSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	normalized, err := normalizeEmail(to)
	if err != nil {
		return &smtp.SMTPError{Code: 553, Message: "invalid recipient address"}
	}
	username, domain, ok := splitEmail(normalized)
	if !ok || !validMailboxDomain(domain) {
		return &smtp.SMTPError{Code: 550, Message: "recipient is not local"}
	}
	if !s.app.localUserActive(username, domain) {
		return &smtp.SMTPError{Code: 550, Message: "recipient is not registered"}
	}
	s.rcpts = append(s.rcpts, normalized)
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	if len(s.rcpts) == 0 {
		return &smtp.SMTPError{Code: 554, Message: "no recipients"}
	}
	data, err := readLimited(r, s.app.cfg.MaxMessageBytes)
	if err != nil {
		return &smtp.SMTPError{Code: 552, Message: err.Error()}
	}

	parsed, err := parseMIMEMessage(data)
	if err != nil {
		return &smtp.SMTPError{Code: 554, Message: "could not parse message"}
	}
	from := s.from
	if parsed.From != "" {
		from = parsed.From
	}

	for _, rcpt := range s.rcpts {
		username, domain, _ := splitEmail(rcpt)
		message := Message{
			MessageID:  newMessageID(),
			From:       from,
			To:         rcpt,
			Subject:    parsed.Subject,
			BodyText:   parsed.BodyText,
			ReceivedAt: s.app.now().UTC(),
		}
		if s.app.deliver(mailboxKey(username, domain), message) != deliverOK {
			return &smtp.SMTPError{Code: 550, Message: "message rejected by recipient inbox policy or recipient expired"}
		}
	}
	return nil
}

func (a *App) localUserActive(username, domain string) bool {
	now := a.now()
	a.mu.RLock()
	defer a.mu.RUnlock()
	user, ok := a.users[mailboxKey(username, domain)]
	return ok && user.ExpiresAt.After(now)
}

type parsedEmail struct {
	From     string
	Subject  string
	BodyText string
}

func parseMIMEMessage(data []byte) (parsedEmail, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return parsedEmail{}, err
	}

	var parsed parsedEmail
	if from := msg.Header.Get("From"); from != "" {
		if normalized, err := normalizeEmail(from); err == nil {
			parsed.From = normalized
		}
	}
	parsed.Subject = decodeHeader(msg.Header.Get("Subject"))

	body, err := extractText(msg.Body, msg.Header.Get("Content-Type"), msg.Header.Get("Content-Transfer-Encoding"))
	if err != nil {
		return parsedEmail{}, err
	}
	parsed.BodyText = strings.TrimSpace(body)
	return parsed, nil
}

func decodeHeader(value string) string {
	decoded, err := new(mime.WordDecoder).DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func extractText(r io.Reader, contentType string, transferEncoding string) (string, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "text/plain"
	}

	if strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return "", errors.New("multipart message missing boundary")
		}
		mr := multipart.NewReader(r, boundary)
		var htmlCandidate string
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return "", err
			}
			partText, err := extractText(part, part.Header.Get("Content-Type"), part.Header.Get("Content-Transfer-Encoding"))
			if err != nil {
				continue
			}
			partMediaType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
			partMediaType = strings.ToLower(partMediaType)
			if partMediaType == "text/plain" && strings.TrimSpace(partText) != "" {
				return partText, nil
			}
			if strings.TrimSpace(partText) != "" && htmlCandidate == "" {
				htmlCandidate = partText
			}
		}
		return htmlCandidate, nil
	}

	data, err := readDecodedBody(r, transferEncoding)
	if err != nil {
		return "", err
	}

	switch strings.ToLower(mediaType) {
	case "text/html":
		text, err := html2text.FromString(string(data), html2text.Options{TextOnly: true})
		if err != nil {
			return "", err
		}
		return text, nil
	case "text/plain", "":
		return string(data), nil
	default:
		return "", nil
	}
}

func readDecodedBody(r io.Reader, transferEncoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(transferEncoding)) {
	case "base64":
		return io.ReadAll(base64.NewDecoder(base64.StdEncoding, r))
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(r))
	default:
		return io.ReadAll(r)
	}
}
