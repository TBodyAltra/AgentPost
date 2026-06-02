package main

import (
	"strings"
	"time"
)

const (
	maxMessageLogEntries   = 1000
	dashboardMessageLogMax = 200
)

// MessageLogEntry records a message delivered to a recipient queue.
// ReceivedAt is set when the recipient polls with GET /api/v1/messages.
type MessageLogEntry struct {
	At         time.Time  `json:"at"`
	From       string     `json:"from"`
	To         string     `json:"to"`
	Subject    string     `json:"subject"`
	BodyText   string     `json:"body_text"`
	MessageID  string     `json:"message_id"`
	ReceivedAt *time.Time `json:"received_at,omitempty"`
}

type dashboardMessageLogEntry struct {
	At         time.Time  `json:"at"`
	From       string     `json:"from"`
	To         string     `json:"to"`
	Subject    string     `json:"subject"`
	BodyText   string     `json:"body_text,omitempty"`
	MessageID  string     `json:"message_id"`
	ReceivedAt *time.Time `json:"received_at,omitempty"`
}

func (a *App) recordMessageDelivered(message Message, recipientMailbox string) {
	now := message.ReceivedAt.UTC()
	if now.IsZero() {
		now = a.now().UTC()
	}
	a.appendMessageLog(MessageLogEntry{
		At:        now,
		From:      strings.ToLower(strings.TrimSpace(message.From)),
		To:        strings.ToLower(strings.TrimSpace(recipientMailbox)),
		Subject:   strings.TrimSpace(message.Subject),
		BodyText:  strings.TrimSpace(message.BodyText),
		MessageID: message.MessageID,
	})
}

func (a *App) markMessagesReceived(mailbox string, messages []Message) {
	mailbox = strings.ToLower(strings.TrimSpace(mailbox))
	now := a.now().UTC()
	for _, m := range messages {
		msgID := strings.TrimSpace(m.MessageID)
		if msgID == "" {
			continue
		}
		for i := len(a.messageLog) - 1; i >= 0; i-- {
			e := &a.messageLog[i]
			if e.MessageID != msgID || e.To != mailbox {
				continue
			}
			if e.ReceivedAt == nil || e.ReceivedAt.IsZero() {
				at := now
				e.ReceivedAt = &at
			}
			break
		}
	}
}

func (a *App) appendMessageLog(entry MessageLogEntry) {
	if entry.MessageID == "" {
		return
	}
	a.messageLog = append(a.messageLog, entry)
	if len(a.messageLog) > maxMessageLogEntries {
		trim := len(a.messageLog) - maxMessageLogEntries
		a.messageLog = append([]MessageLogEntry(nil), a.messageLog[trim:]...)
	}
}

func (a *App) clearMessageLog() {
	a.messageLog = nil
}

func dashboardMessageLogSnapshot(log []MessageLogEntry) []dashboardMessageLogEntry {
	if len(log) == 0 {
		return nil
	}
	start := 0
	if len(log) > dashboardMessageLogMax {
		start = len(log) - dashboardMessageLogMax
	}
	out := make([]dashboardMessageLogEntry, 0, len(log)-start)
	for i := len(log) - 1; i >= start; i-- {
		e := log[i]
		var receivedAt *time.Time
		if e.ReceivedAt != nil && !e.ReceivedAt.IsZero() {
			t := e.ReceivedAt.UTC()
			receivedAt = &t
		}
		out = append(out, dashboardMessageLogEntry{
			At:         e.At.UTC(),
			From:       e.From,
			To:         e.To,
			Subject:    dashboardTruncateSubject(e.Subject),
			BodyText:   dashboardTruncateBody(e.BodyText),
			MessageID:  e.MessageID,
			ReceivedAt: receivedAt,
		})
	}
	return out
}
