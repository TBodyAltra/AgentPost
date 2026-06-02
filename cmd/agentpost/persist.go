package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const mailboxPersistVersion = 1

type mailboxPersistFile struct {
	Version   int                `json:"version"`
	Mailboxes []persistedMailbox `json:"mailboxes"`
}

type persistedMailbox struct {
	Username     string       `json:"username"`
	Domain       string       `json:"domain"`
	PublicKey    string       `json:"public_key"`
	ExpiresAt    time.Time    `json:"expires_at"`
	RegisteredAt time.Time    `json:"registered_at"`
	LastPolledAt time.Time    `json:"last_polled_at,omitempty"`
	Profile      AgentProfile `json:"profile,omitempty"`
	InboxPolicy  InboxPolicy  `json:"inbox_policy,omitempty"`
}

func defaultDataDir() string {
	if v := strings.TrimSpace(os.Getenv("AGENTPOST_DATA_DIR")); v != "" {
		return v
	}
	return ".agentpost/data"
}

func storageDescription(dataDir string) string {
	if strings.TrimSpace(dataDir) != "" {
		return "registered mailboxes on disk; queues and message log in-memory"
	}
	return "in-memory"
}

func (a *App) mailboxesPersistPath() string {
	dir := strings.TrimSpace(a.cfg.DataDir)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "mailboxes.json")
}

func (a *App) loadPersistedUsers() {
	path := a.mailboxesPersistPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Printf("[agentpost] load mailboxes %s: %v", path, err)
		return
	}
	var file mailboxPersistFile
	if err := json.Unmarshal(data, &file); err != nil {
		log.Printf("[agentpost] parse mailboxes %s: %v", path, err)
		return
	}
	if file.Version != 0 && file.Version != mailboxPersistVersion {
		log.Printf("[agentpost] unsupported mailboxes version %d in %s", file.Version, path)
		return
	}

	now := a.now().UTC()
	loaded := 0
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, rec := range file.Mailboxes {
		user, err := userFromPersisted(rec)
		if err != nil {
			log.Printf("[agentpost] skip persisted mailbox: %v", err)
			continue
		}
		if !user.ExpiresAt.After(now) {
			continue
		}
		mailbox := userMailbox(user)
		a.users[mailbox] = user
		a.messages[mailbox] = nil
		a.ensureLimiterLocked(mailbox)
		loaded++
	}
	if loaded > 0 {
		log.Printf("[agentpost] restored %d mailbox(es) from %s", loaded, path)
	}
}

func userFromPersisted(rec persistedMailbox) (*User, error) {
	username := strings.ToLower(strings.TrimSpace(rec.Username))
	domain := strings.ToLower(strings.TrimSpace(rec.Domain))
	if !validUsername(username) || !validMailboxDomain(domain) {
		return nil, errors.New("invalid username or domain")
	}
	publicKey, err := hex.DecodeString(strings.TrimSpace(rec.PublicKey))
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("invalid public_key")
	}
	if rec.ExpiresAt.IsZero() || rec.RegisteredAt.IsZero() {
		return nil, errors.New("missing registration timestamps")
	}
	return &User{
		Username:     username,
		Domain:       domain,
		PublicKey:    append(ed25519.PublicKey(nil), publicKey...),
		ExpiresAt:    rec.ExpiresAt.UTC(),
		RegisteredAt: rec.RegisteredAt.UTC(),
		LastPolledAt: rec.LastPolledAt.UTC(),
		Profile:      rec.Profile,
		InboxPolicy:  rec.InboxPolicy,
	}, nil
}

func (a *App) ensureLimiterLocked(mailbox string) {
	if _, ok := a.limiters[mailbox]; !ok {
		a.limiters[mailbox] = rate.NewLimiter(rate.Every(time.Minute/2), 2)
	}
}

func (a *App) savePersistedUsers() {
	path := a.mailboxesPersistPath()
	if path == "" {
		return
	}

	now := a.now().UTC()
	a.mu.RLock()
	records := make([]persistedMailbox, 0, len(a.users))
	for _, user := range a.users {
		if user == nil || !user.ExpiresAt.After(now) {
			continue
		}
		records = append(records, persistedMailbox{
			Username:     user.Username,
			Domain:       user.Domain,
			PublicKey:    hex.EncodeToString(user.PublicKey),
			ExpiresAt:    user.ExpiresAt.UTC(),
			RegisteredAt: user.RegisteredAt.UTC(),
			LastPolledAt: user.LastPolledAt.UTC(),
			Profile:      user.Profile,
			InboxPolicy:  user.InboxPolicy,
		})
	}
	a.mu.RUnlock()

	if err := writeMailboxPersistFile(path, records); err != nil {
		log.Printf("[agentpost] save mailboxes %s: %v", path, err)
	}
}

func writeMailboxPersistFile(path string, records []persistedMailbox) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(mailboxPersistFile{
		Version:   mailboxPersistVersion,
		Mailboxes: records,
	}, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (a *App) persistUsersAsync() {
	go a.savePersistedUsers()
}
