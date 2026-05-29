package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	stdsmtp "net/smtp"
	"sync"
	"testing"
	"time"

	smtp "github.com/emersion/go-smtp"
)

func TestSMTPServerEndToEndOverNetwork(t *testing.T) {
	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "127.0.0.1:0",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	handler := app.routes()
	privateKey := registerTestUser(t, handler, "smtp_target", "agent.test", "", nil)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := newSMTPServer(app)
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil && err != smtp.ErrServerClosed {
			t.Errorf("shutdown SMTP server: %v", err)
		}
		if err := <-errCh; err != nil {
			t.Errorf("SMTP server returned unexpected error: %v", err)
		}
	})

	client, err := stdsmtp.Dial(listener.Addr().String())
	if err != nil {
		t.Fatalf("dial SMTP server: %v", err)
	}
	defer client.Close()
	if err := client.Hello("client.agent.test"); err != nil {
		t.Fatalf("HELO failed: %v", err)
	}
	if err := client.Mail("human@agent.test"); err != nil {
		t.Fatalf("MAIL FROM failed: %v", err)
	}
	if err := client.Rcpt("smtp_target@agent.test"); err != nil {
		t.Fatalf("RCPT TO failed: %v", err)
	}
	writer, err := client.Data()
	if err != nil {
		t.Fatalf("DATA command failed: %v", err)
	}
	raw := "From: Human <human@agent.test>\r\n" +
		"To: smtp_target@agent.test\r\n" +
		"Subject: Network SMTP\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"delivered through a real SMTP connection\r\n"
	if _, err := io.WriteString(writer, raw); err != nil {
		t.Fatalf("write SMTP DATA body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close SMTP DATA body: %v", err)
	}
	if err := client.Quit(); err != nil {
		t.Fatalf("QUIT failed: %v", err)
	}

	messages := pollTestMessages(t, handler, "smtp_target@agent.test", privateKey)
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	message := messages[0]
	if message.From != "human@agent.test" || message.Subject != "Network SMTP" || message.BodyText != "delivered through a real SMTP connection" {
		t.Fatalf("unexpected SMTP message: %+v", message)
	}
}

func TestConcurrentHTTPRegisterSendAndPoll(t *testing.T) {
	const users = 24

	app := NewApp(Config{
		Domain:          "agent.test",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
	})
	handler := app.routes()

	privateKeys := make([]ed25519.PrivateKey, users)
	for i := range users {
		username := fmt.Sprintf("bot_%02d", i)
		privateKeys[i] = registerTestUser(t, handler, username, "agent.test", fmt.Sprintf("192.0.2.%d:12345", i+1), nil)
	}

	var wg sync.WaitGroup
	errs := make(chan string, users)
	for i := range users {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			from := fmt.Sprintf("bot_%02d@agent.test", i)
			to := fmt.Sprintf("bot_%02d@agent.test", (i+1)%users)
			body := mustJSON(t, sendRequest{
				To:      to,
				Subject: fmt.Sprintf("msg-%02d", i),
				Body:    fmt.Sprintf("hello from %s", from),
			})
			req := signedRequest(t, http.MethodPost, "/api/v1/send", body, from, privateKeys[i])
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				errs <- fmt.Sprintf("send %s -> %s status = %d, body = %s", from, to, resp.Code, resp.Body.String())
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if t.Failed() {
		t.FailNow()
	}

	errs = make(chan string, users)
	for i := range users {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			email := fmt.Sprintf("bot_%02d@agent.test", i)
			req := signedRequest(t, http.MethodGet, "/api/v1/messages", nil, email, privateKeys[i])
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				errs <- fmt.Sprintf("poll %s status = %d, body = %s", email, resp.Code, resp.Body.String())
				return
			}
			var got messagesResponse
			if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
				errs <- fmt.Sprintf("decode %s messages response: %v", email, err)
				return
			}
			messages := got.Messages
			if len(messages) != 1 {
				errs <- fmt.Sprintf("%s message count = %d, want 1: %+v", email, len(messages), messages)
				return
			}
			wantSender := fmt.Sprintf("bot_%02d@agent.test", (i-1+users)%users)
			if messages[0].From != wantSender {
				errs <- fmt.Sprintf("%s message from = %q, want %q", email, messages[0].From, wantSender)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func registerTestUser(t *testing.T, handler http.Handler, username, domain, remoteAddr string, policy *InboxPolicy) ed25519.PrivateKey {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	body := mustJSON(t, registerRequest{
		Username:    username,
		Domain:      domain,
		PublicKey:   hex.EncodeToString(publicKey),
		TTLSeconds:  3600,
		InboxPolicy: policy,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("register %s status = %d, body = %s", username, resp.Code, resp.Body.String())
	}
	return privateKey
}

func pollTestMessages(t *testing.T, handler http.Handler, email string, privateKey ed25519.PrivateKey) []Message {
	t.Helper()
	req := signedRequest(t, http.MethodGet, "/api/v1/messages", nil, email, privateKey)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("poll %s status = %d, body = %s", email, resp.Code, resp.Body.String())
	}
	var got messagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode messages response: %v", err)
	}
	return got.Messages
}
