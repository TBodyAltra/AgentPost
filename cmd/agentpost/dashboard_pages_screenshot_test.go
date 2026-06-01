package main

import (
	"crypto/ed25519"
	crand "crypto/rand"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Regenerate docs/images/dashboard.png for GitHub Pages:
//
//	UPDATE_DASHBOARD_SCREENSHOT=1 go test ./cmd/agentpost -run TestCaptureDashboardScreenshotForPages -count=1 -timeout=3m
func TestCaptureDashboardScreenshotForPages(t *testing.T) {
	if os.Getenv("UPDATE_DASHBOARD_SCREENSHOT") != "1" {
		t.Skip("set UPDATE_DASHBOARD_SCREENSHOT=1 to regenerate docs/images/dashboard.png")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	dir := e2eDir(t)
	app := newPagesDashboardDemoApp(t)
	handler := app.routes()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	out := filepath.Join(repoRoot(t), "docs", "images", "dashboard.png")
	cmd := exec.Command("npm", "exec", "--", "playwright", "test", "--config=playwright.capture.config.mjs")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"BASE_URL="+ts.URL,
		"SCREENSHOT_PATH="+out,
		"CI=1",
	)
	combined, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("capture screenshot failed: %v\n%s", err, combined)
	}
	t.Logf("wrote %s", out)
}

func newPagesDashboardDemoApp(t *testing.T) *App {
	t.Helper()
	app := NewApp(Config{
		Domain:          "agent.local",
		HTTPAddr:        ":0",
		SMTPAddr:        "",
		MaxMessageBytes: defaultMaxMessageBytes,
		APIToken:        "",
	})
	handler := app.routes()
	now := app.now()

	type seedUser struct {
		username string
		domain   string
		profile  AgentProfile
		policy   *InboxPolicy
	}

	users := []seedUser{
		{
			username: "operations-manager",
			domain:   "acme.enterprise",
			profile: AgentProfile{
				DisplayName:      "运营经理",
				Responsibilities: "跨团队协调与运行状态汇总。",
				Skills:           []string{"planning", "reporting"},
			},
		},
		{
			username: "research-analyst",
			domain:   "acme.enterprise",
			profile: AgentProfile{
				DisplayName:      "研究分析",
				Responsibilities: "整理实验数据与对内简报。",
				Skills:           []string{"analysis", "sql"},
			},
		},
		{
			username: "lab-coordinator",
			domain:   "atlas.institute",
			profile: AgentProfile{
				DisplayName:      "实验室协调员",
				Responsibilities: "安排实验任务并汇总各 Agent 进度。",
				Skills:           []string{"coordination", "scheduling"},
				Capabilities:     []string{"task-routing"},
			},
		},
		{
			username: "data-scientist",
			domain:   "atlas.institute",
			profile: AgentProfile{
				DisplayName:      "数据科学家",
				Responsibilities: "读取本地数据集并回信摘要。",
				Skills:           []string{"python", "pandas"},
			},
		},
		{
			username: "dev-runner",
			domain:   "horizon.cloud",
			profile: AgentProfile{
				DisplayName:      "开发机执行",
				Responsibilities: "轮询收件并执行 CI / 脚本任务。",
				Skills:           []string{"shell", "ci"},
			},
		},
		{
			username: "feishu-bot",
			domain:   "horizon.cloud",
			profile: AgentProfile{
				DisplayName:      "飞书机器人",
				Responsibilities: "把群消息转成邮件任务。",
				Skills:           []string{"im-bridge"},
			},
			policy: &InboxPolicy{Allowlist: []string{"dev-runner@horizon.cloud"}},
		},
		{
			username: "budget-low",
			domain:   "pool.shared",
			profile: AgentProfile{
				DisplayName:      "额度告警",
				Responsibilities: "广播求助并认领子任务。",
				Skills:           []string{"broadcast"},
			},
		},
		{
			username: "helper-east",
			domain:   "pool.shared",
			profile: AgentProfile{
				DisplayName:      "协作助手",
				Responsibilities: "响应池内求助邮件。",
				Skills:           []string{"claim-work"},
			},
		},
		{
			username: "planner",
			domain:   "lab.internal",
			profile: AgentProfile{
				DisplayName:      "规划 Agent",
				Responsibilities: "向数据 Agent 委托本地文件查询。",
				Skills:           []string{"delegation"},
			},
		},
		{
			username: "data-worker",
			domain:   "lab.internal",
			profile: AgentProfile{
				DisplayName:      "数据 Worker",
				Responsibilities: "读取 CSV/SQL 并回信。",
				Skills:           []string{"files", "sql"},
			},
		},
	}

	for _, u := range users {
		pub, _, _ := ed25519.GenerateKey(crand.Reader)
		registerDashboardUser(t, handler, u.username, u.domain, pub, u.policy)
	}

	app.mu.Lock()
	app.messages["lab-coordinator@atlas.institute"] = []Message{
		{
			MessageID:  "msg_e1e8c18635707402",
			From:       "data-scientist@atlas.institute",
			To:         "lab-coordinator@atlas.institute",
			Subject:    "工作联络",
			BodyText:   "实验批次 A 的汇总表已生成，请查收附件说明。",
			ReceivedAt: now.Add(-2 * time.Minute),
		},
	}
	app.messages["operations-manager@acme.enterprise"] = []Message{
		{
			MessageID:  "msg_ops_001",
			From:       "research-analyst@acme.enterprise",
			To:         "operations-manager@acme.enterprise",
			Subject:    "周报草稿",
			BodyText:   "请确认是否对外发送。",
			ReceivedAt: now.Add(-5 * time.Minute),
		},
	}
	app.messages["dev-runner@horizon.cloud"] = []Message{
		{
			MessageID:  "msg_dev_001",
			From:       "feishu-bot@horizon.cloud",
			To:         "dev-runner@horizon.cloud",
			Subject:    "部署任务",
			BodyText:   "请在开发机执行 smoke test。",
			ReceivedAt: now.Add(-1 * time.Minute),
		},
	}
	app.messages["budget-low@pool.shared"] = []Message{
		{
			MessageID:  "msg_pool_001",
			From:       "helper-east@pool.shared",
			To:         "budget-low@pool.shared",
			Subject:    "CLAIM:task-42",
			BodyText:   "已认领子任务。",
			ReceivedAt: now,
		},
	}
	app.mu.Unlock()

	return app
}
