package main

import (
	"strings"
	"time"
)

const (
	maxMessageLogEntries     = 1000
	dashboardMessageLogMax   = 200
	messageLogEventDelivered = "delivered"
	messageLogEventReceived  = "received"
)

// MessageLogEntry records a send (delivered to queue) or receive (polled by agent).
type MessageLogEntry struct {
	Event     string    `json:"event"`
	At        time.Time `json:"at"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	MessageID string    `json:"message_id"`
}

type dashboardMessageLogEntry struct {
	Event     string    `json:"event"`
	At        time.Time `json:"at"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	MessageID string    `json:"message_id"`
}

func (a *App) recordMessageDelivered(message Message, recipientMailbox string) {
	a.appendMessageLog(MessageLogEntry{
		Event:     messageLogEventDelivered,
		At:        message.ReceivedAt.UTC(),
		From:      strings.ToLower(strings.TrimSpace(message.From)),
		To:        strings.ToLower(strings.TrimSpace(recipientMailbox)),
		Subject:   strings.TrimSpace(message.Subject),
		MessageID: message.MessageID,
	})
}

func (a *App) recordMessagesReceived(mailbox string, messages []Message) {
	mailbox = strings.ToLower(strings.TrimSpace(mailbox))
	now := a.now().UTC()
	for _, m := range messages {
		at := m.ReceivedAt.UTC()
		if at.IsZero() {
			at = now
		}
		a.appendMessageLog(MessageLogEntry{
			Event:     messageLogEventReceived,
			At:        at,
			From:      strings.ToLower(strings.TrimSpace(m.From)),
			To:        mailbox,
			Subject:   strings.TrimSpace(m.Subject),
			MessageID: m.MessageID,
		})
	}
}

func (a *App) appendMessageLog(entry MessageLogEntry) {
	if entry.Event == "" || entry.MessageID == "" {
		return
	}
	a.messageLog = append(a.messageLog, entry)
	if len(a.messageLog) > maxMessageLogEntries {
		trim := len(a.messageLog) - maxMessageLogEntries
		a.messageLog = append([]MessageLogEntry(nil), a.messageLog[trim:]...)
	}
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
		out = append(out, dashboardMessageLogEntry{
			Event:     e.Event,
			At:        e.At.UTC(),
			From:      e.From,
			To:        e.To,
			Subject:   dashboardTruncateSubject(e.Subject),
			MessageID: e.MessageID,
		})
	}
	return out
}
