package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
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
)

type Config struct {
	Domain             string `yaml:"domain"`
	HTTPAddr           string `yaml:"http_addr"`
	SMTPAddr           string `yaml:"smtp_addr"`
	AllowExternalRelay bool   `yaml:"allow_external_relay"`
	MaxMessageBytes    int64  `yaml:"max_message_bytes"`
}

type App struct {
	cfg Config
	now func() time.Time

	mu       sync.RWMutex
	users    map[string]*User
	messages map[string][]Message
	limiters map[string]*rate.Limiter
}

type User struct {
	Username  string
	PublicKey ed25519.PublicKey
	ExpiresAt time.Time
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
	Username   string `json:"username"`
	PublicKey  string `json:"public_key"`
	TTLSeconds int64  `json:"ttl_seconds"`
}

type registerResponse struct {
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expires_at"`
	Status    string    `json:"status"`
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
		log.Printf("AgentPost HTTP listening on %s for domain %s", cfg.HTTPAddr, cfg.Domain)
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
	applyEnvOverrides(&cfg)
	return cfg, nil
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
}

func NewApp(cfg Config) *App {
	cfg.Domain = strings.ToLower(strings.TrimSpace(cfg.Domain))
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
	mux.HandleFunc("/api/v1/send", a.handleSend)
	mux.HandleFunc("/api/v1/messages", a.handleMessages)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
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

	now := a.now().UTC()
	expiresAt := now.Add(time.Duration(ttl) * time.Second)

	a.mu.Lock()
	if existing, ok := a.users[username]; ok && existing.ExpiresAt.After(now) {
		a.mu.Unlock()
		writeJSON(w, http.StatusConflict, errorResponse{Error: "username is already registered"})
		return
	}
	a.users[username] = &User{
		Username:  username,
		PublicKey: append(ed25519.PublicKey(nil), publicKey...),
		ExpiresAt: expiresAt,
	}
	a.limiters[username] = rate.NewLimiter(rate.Every(time.Minute/2), 2)
	a.mu.Unlock()

	writeJSON(w, http.StatusCreated, registerResponse{
		Email:     a.emailFor(username),
		ExpiresAt: expiresAt,
		Status:    "active",
	})
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
	limiter := a.limiters[user.Username]
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

	if domain != a.cfg.Domain {
		if !a.cfg.AllowExternalRelay {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "external relay is disabled"})
			return
		}
		writeJSON(w, http.StatusNotImplemented, errorResponse{Error: "external relay is not implemented in the MVP"})
		return
	}

	message := Message{
		MessageID:  newMessageID(),
		From:       a.emailFor(user.Username),
		To:         to,
		Subject:    strings.TrimSpace(req.Subject),
		BodyText:   strings.TrimSpace(req.Body),
		ReceivedAt: a.now().UTC(),
	}

	if !a.deliver(username, message) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "recipient is not registered or has expired"})
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
	messages := append([]Message(nil), a.messages[user.Username]...)
	a.messages[user.Username] = nil
	a.mu.Unlock()

	writeJSON(w, http.StatusOK, messagesResponse{Messages: messages})
}

func (a *App) authenticate(r *http.Request, body []byte) (*User, int, error) {
	username := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Agent-Username")))
	if !validUsername(username) {
		return nil, http.StatusUnauthorized, errors.New("missing or invalid X-Agent-Username")
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
	user, ok := a.users[username]
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

func (a *App) deliver(username string, message Message) bool {
	username = strings.ToLower(strings.TrimSpace(username))
	now := a.now()

	a.mu.Lock()
	defer a.mu.Unlock()
	user, ok := a.users[username]
	if !ok || !user.ExpiresAt.After(now) {
		return false
	}
	a.messages[username] = append(a.messages[username], message)
	return true
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
	for username, user := range a.users {
		if !user.ExpiresAt.After(now) {
			delete(a.users, username)
			delete(a.messages, username)
			delete(a.limiters, username)
		}
	}
}

func (a *App) emailFor(username string) string {
	return username + "@" + a.cfg.Domain
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
	if !ok || domain != s.app.cfg.Domain {
		return &smtp.SMTPError{Code: 550, Message: "recipient is not local"}
	}
	if !s.app.localUserActive(username) {
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
		username, _, _ := splitEmail(rcpt)
		message := Message{
			MessageID:  newMessageID(),
			From:       from,
			To:         rcpt,
			Subject:    parsed.Subject,
			BodyText:   parsed.BodyText,
			ReceivedAt: s.app.now().UTC(),
		}
		if !s.app.deliver(username, message) {
			return &smtp.SMTPError{Code: 550, Message: "recipient expired"}
		}
	}
	return nil
}

func (a *App) localUserActive(username string) bool {
	now := a.now()
	a.mu.RLock()
	defer a.mu.RUnlock()
	user, ok := a.users[username]
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
