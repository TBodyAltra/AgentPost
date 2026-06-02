package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistMailboxesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	pub, _, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(pub)

	now := time.Now().UTC().Truncate(time.Second)
	app1 := NewApp(Config{Domain: "agent.local", DataDir: dir})
	app1.now = func() time.Time { return now }

	mailbox := "worker@agent.local"
	app1.mu.Lock()
	app1.users[mailbox] = &User{
		Username:     "worker",
		Domain:       "agent.local",
		PublicKey:    append(ed25519.PublicKey(nil), pub...),
		ExpiresAt:    now.Add(time.Hour),
		RegisteredAt: now,
		Profile:      AgentProfile{DisplayName: "Worker"},
		InboxPolicy:  defaultInboxPolicy(),
	}
	app1.messages[mailbox] = []Message{{MessageID: "msg_1", From: "other@agent.local", Subject: "hi"}}
	app1.mu.Unlock()
	app1.savePersistedUsers()

	app2 := NewApp(Config{Domain: "agent.local", DataDir: dir})
	app2.now = app1.now

	app2.mu.RLock()
	user, ok := app2.users[mailbox]
	queue := append([]Message(nil), app2.messages[mailbox]...)
	app2.mu.RUnlock()

	if !ok {
		t.Fatal("expected mailbox restored after restart")
	}
	if user.Profile.DisplayName != "Worker" {
		t.Fatalf("profile = %+v", user.Profile)
	}
	if len(queue) != 0 {
		t.Fatalf("queue should not be persisted, got %d messages", len(queue))
	}

	path := filepath.Join(dir, "mailboxes.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persist file: %v", err)
	}
	var file mailboxPersistFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("parse persist file: %v", err)
	}
	if len(file.Mailboxes) != 1 || file.Mailboxes[0].PublicKey != pubHex {
		t.Fatalf("persist file = %+v", file)
	}
}

func TestPersistMailboxesSkipsExpired(t *testing.T) {
	dir := t.TempDir()
	pub, _, _ := ed25519.GenerateKey(nil)
	now := time.Now().UTC().Truncate(time.Second)

	path := filepath.Join(dir, "mailboxes.json")
	payload, err := json.Marshal(mailboxPersistFile{
		Version: mailboxPersistVersion,
		Mailboxes: []persistedMailbox{
			{
				Username:     "old",
				Domain:       "agent.local",
				PublicKey:    hex.EncodeToString(pub),
				ExpiresAt:    now.Add(-time.Minute),
				RegisteredAt: now.Add(-time.Hour),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(Config{Domain: "agent.local", DataDir: dir})
	app.now = func() time.Time { return now }
	if len(app.users) != 0 {
		t.Fatalf("expired mailbox should not load, got %d users", len(app.users))
	}
}

func TestStorageDescription(t *testing.T) {
	if got := storageDescription(".agentpost/data"); got == "in-memory" {
		t.Fatalf("expected disk description, got %q", got)
	}
	if got := storageDescription(""); got != "in-memory" {
		t.Fatalf("got %q", got)
	}
}
