package main

import (
	"embed"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"time"
)

//go:embed web/dashboard/*
var dashboardFS embed.FS

type dashboardResponse struct {
	GeneratedAt time.Time                  `json:"generated_at"`
	Gateway     dashboardGateway           `json:"gateway"`
	Domains     []dashboardDomain          `json:"domains"`
	Mailboxes   []dashboardMailbox         `json:"mailboxes"`
	Links       []dashboardLink            `json:"links"`
	MessageLog  []dashboardMessageLogEntry `json:"message_log"`
}

type dashboardGateway struct {
	DefaultDomain       string `json:"default_domain"`
	ServerURL           string `json:"server_url"`
	GatewayToken        bool   `json:"gateway_token_required"`
	SMTPEnabled         bool   `json:"smtp_inbound_enabled"`
	ActiveMailboxes     int    `json:"active_mailboxes"`
	DomainCount         int    `json:"domain_count"`
	TotalQueuedMessages int    `json:"total_queued_messages"`
	MailboxesWithQueue  int    `json:"mailboxes_with_queue"`
}

type dashboardMessagePreview struct {
	MessageID  string    `json:"message_id"`
	From       string    `json:"from"`
	Subject    string    `json:"subject"`
	ReceivedAt time.Time `json:"received_at"`
}

type dashboardDomain struct {
	Domain       string   `json:"domain"`
	IsDefault    bool     `json:"is_default"`
	MailboxCount int      `json:"mailbox_count"`
	Mailboxes    []string `json:"mailboxes"`
}

type dashboardMailbox struct {
	Username            string                    `json:"username"`
	Domain              string                    `json:"domain"`
	Email               string                    `json:"email"`
	ExpiresAt           time.Time                 `json:"expires_at"`
	RegisteredAt        time.Time                 `json:"registered_at"`
	TTLRemainingSeconds int64                     `json:"ttl_remaining_seconds"`
	QueuedMessages      int                       `json:"queued_messages"`
	Queue               []dashboardMessagePreview `json:"queue,omitempty"`
	Profile             AgentProfile              `json:"profile,omitempty"`
	InboxPolicy         InboxPolicy               `json:"inbox_policy"`
}

type dashboardLink struct {
	From          string `json:"from"`
	To            string `json:"to"`
	FromDomain    string `json:"from_domain"`
	ToDomain      string `json:"to_domain"`
	ForwardStatus string `json:"forward_status"` // From -> To delivery
	ReverseStatus string `json:"reverse_status"` // To -> From delivery
	SameDomain    bool   `json:"same_domain"`
}

func (a *App) handleDashboardAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, a.buildDashboardSnapshot(r))
}

func (a *App) buildDashboardSnapshot(r *http.Request) dashboardResponse {
	now := a.now().UTC()
	serverURL, _ := a.resolveServerURL(r)

	a.mu.RLock()
	defer a.mu.RUnlock()

	mailboxes := make([]dashboardMailbox, 0, len(a.users))
	domainMailboxes := make(map[string][]string)
	activeEmails := make([]string, 0, len(a.users))
	totalQueued := 0
	mailboxesWithQueue := 0

	for _, user := range a.users {
		if !user.ExpiresAt.After(now) {
			continue
		}
		email := userMailbox(user)
		activeEmails = append(activeEmails, email)
		ttlRemaining := int64(user.ExpiresAt.Sub(now).Seconds())
		if ttlRemaining < 0 {
			ttlRemaining = 0
		}
		queue := a.messages[email]
		qn := len(queue)
		totalQueued += qn
		if qn > 0 {
			mailboxesWithQueue++
		}
		mailboxes = append(mailboxes, dashboardMailbox{
			Username:            user.Username,
			Domain:              user.Domain,
			Email:               email,
			ExpiresAt:           user.ExpiresAt.UTC(),
			RegisteredAt:        user.RegisteredAt.UTC(),
			TTLRemainingSeconds: ttlRemaining,
			QueuedMessages:      qn,
			Queue:               dashboardQueuePreviews(queue),
			Profile:             user.Profile,
			InboxPolicy:         user.InboxPolicy,
		})
		domainMailboxes[user.Domain] = append(domainMailboxes[user.Domain], email)
	}

	sort.Slice(mailboxes, func(i, j int) bool {
		if mailboxes[i].Domain != mailboxes[j].Domain {
			return mailboxes[i].Domain < mailboxes[j].Domain
		}
		return mailboxes[i].Username < mailboxes[j].Username
	})
	sort.Strings(activeEmails)

	domains := make([]dashboardDomain, 0, len(domainMailboxes))
	for domain, emails := range domainMailboxes {
		sort.Strings(emails)
		domains = append(domains, dashboardDomain{
			Domain:       domain,
			IsDefault:    domain == a.cfg.Domain,
			MailboxCount: len(emails),
			Mailboxes:    emails,
		})
	}
	sort.Slice(domains, func(i, j int) bool {
		if domains[i].IsDefault != domains[j].IsDefault {
			return domains[i].IsDefault
		}
		return domains[i].Domain < domains[j].Domain
	})

	userByEmail := make(map[string]*User, len(a.users))
	for _, user := range a.users {
		if user.ExpiresAt.After(now) {
			userByEmail[userMailbox(user)] = user
		}
	}

	links := make([]dashboardLink, 0)
	for i := 0; i < len(activeEmails); i++ {
		for j := i + 1; j < len(activeEmails); j++ {
			a, b := activeEmails[i], activeEmails[j]
			_, aDomain, aOK := splitEmail(a)
			_, bDomain, bOK := splitEmail(b)
			if !aOK || !bOK {
				continue
			}
			recipientB := userByEmail[b]
			recipientA := userByEmail[a]
			forward := "unknown"
			reverse := "unknown"
			if recipientB != nil {
				forward = dashboardDirectedStatus(a, b, recipientB.InboxPolicy)
			}
			if recipientA != nil {
				reverse = dashboardDirectedStatus(b, a, recipientA.InboxPolicy)
			}
			links = append(links, dashboardLink{
				From:          a,
				To:            b,
				FromDomain:    aDomain,
				ToDomain:      bDomain,
				ForwardStatus: forward,
				ReverseStatus: reverse,
				SameDomain:    aDomain == bDomain,
			})
		}
	}

	return dashboardResponse{
		GeneratedAt: now,
		Gateway: dashboardGateway{
			DefaultDomain:       a.cfg.Domain,
			ServerURL:           serverURL,
			GatewayToken:        strings.TrimSpace(a.cfg.APIToken) != "",
			SMTPEnabled:         strings.TrimSpace(a.cfg.SMTPAddr) != "",
			ActiveMailboxes:     len(mailboxes),
			DomainCount:         len(domains),
			TotalQueuedMessages: totalQueued,
			MailboxesWithQueue:  mailboxesWithQueue,
		},
		Domains:    domains,
		Mailboxes:  mailboxes,
		Links:      links,
		MessageLog: dashboardMessageLogSnapshot(a.messageLog),
	}
}

func dashboardLinkStatus(from, recipientDomain string, policy InboxPolicy) string {
	from = strings.ToLower(strings.TrimSpace(from))
	_, fromDomain, ok := splitEmail(from)
	if !ok {
		return "unknown"
	}
	if addressListMatches(from, policy.Blocklist, recipientDomain) {
		return "blocked"
	}
	if fromDomain == recipientDomain {
		return "allowed"
	}
	if addressListMatches(from, policy.Allowlist, recipientDomain) {
		return "allowlisted"
	}
	return "cross_domain_blocked"
}

func dashboardDirectedStatus(from, recipientEmail string, policy InboxPolicy) string {
	_, recipientDomain, ok := splitEmail(recipientEmail)
	if !ok {
		return "unknown"
	}
	status := dashboardLinkStatus(from, recipientDomain, policy)
	_, fromDomain, fromOK := splitEmail(from)
	if fromOK && fromDomain == recipientDomain && status == "cross_domain_blocked" {
		return "allowed"
	}
	return status
}

func deliveryStatusPermits(status string) bool {
	return status == "allowed" || status == "allowlisted"
}

const dashboardQueuePreviewMax = 50

func dashboardQueuePreviews(msgs []Message) []dashboardMessagePreview {
	if len(msgs) == 0 {
		return nil
	}
	start := 0
	if len(msgs) > dashboardQueuePreviewMax {
		start = len(msgs) - dashboardQueuePreviewMax
	}
	out := make([]dashboardMessagePreview, 0, len(msgs)-start)
	for _, m := range msgs[start:] {
		out = append(out, dashboardMessagePreview{
			MessageID:  m.MessageID,
			From:       m.From,
			Subject:    dashboardTruncateSubject(m.Subject),
			ReceivedAt: m.ReceivedAt.UTC(),
		})
	}
	return out
}

func dashboardTruncateSubject(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 160 {
		return s
	}
	return s[:157] + "..."
}

func (a *App) dashboardHandler() http.Handler {
	sub, err := fs.Sub(dashboardFS, "web/dashboard")
	if err != nil {
		panic(err)
	}
	files := http.StripPrefix("/dashboard/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		files.ServeHTTP(w, r)
	})
}
