package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStartScriptConfigureScenarios(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantEnv      map[string]string
		wantConfig   Config
		wantCaddy    bool
		caddyContain []string
	}{
		{
			name: "local",
			args: []string{"configure", "--non-interactive", "--scenario", "local", "--http-port", "18080"},
			wantEnv: map[string]string{
				"AGENTPOST_SCENARIO":      "local",
				"AGENTPOST_DOMAIN":        "agent.local",
				"AGENTPOST_HTTP_PORT":     "18080",
				"AGENTPOST_PUBLIC_URL":    "http://127.0.0.1:18080",
				"AGENTPOST_ENABLE_SMTP":   "0",
				"AGENTPOST_ENABLE_CADDY":  "0",
				"AGENTPOST_REQUIRE_TOKEN": "0",
			},
			wantConfig: Config{
				Domain:             "agent.local",
				HTTPAddr:           ":8080",
				SMTPAddr:           "",
				AllowExternalRelay: false,
				MaxMessageBytes:    defaultMaxMessageBytes,
			},
		},
		{
			name: "public IP with SMTP",
			args: []string{"configure", "--non-interactive", "--scenario", "public-ip", "--public-ip", "203.0.113.10", "--domain", "example.domain", "--http-port", "18081", "--smtp"},
			wantEnv: map[string]string{
				"AGENTPOST_SCENARIO":      "public-ip",
				"AGENTPOST_DOMAIN":        "example.domain",
				"AGENTPOST_HTTP_PORT":     "18081",
				"AGENTPOST_PUBLIC_URL":    "http://203.0.113.10:18081",
				"AGENTPOST_ENABLE_SMTP":   "1",
				"AGENTPOST_ENABLE_CADDY":  "0",
				"AGENTPOST_REQUIRE_TOKEN": "1",
			},
			wantConfig: Config{
				Domain:             "example.domain",
				HTTPAddr:           ":8080",
				SMTPAddr:           ":2525",
				AllowExternalRelay: false,
				MaxMessageBytes:    defaultMaxMessageBytes,
			},
		},
		{
			name: "public domain with Caddy",
			args: []string{"configure", "--non-interactive", "--scenario", "public-domain", "--domain", "example.domain", "--smtp"},
			wantEnv: map[string]string{
				"AGENTPOST_SCENARIO":      "public-domain",
				"AGENTPOST_DOMAIN":        "example.domain",
				"AGENTPOST_HTTP_PORT":     "8080",
				"AGENTPOST_PUBLIC_URL":    "https://example.domain",
				"AGENTPOST_ENABLE_SMTP":   "1",
				"AGENTPOST_ENABLE_CADDY":  "1",
				"AGENTPOST_REQUIRE_TOKEN": "1",
			},
			wantConfig: Config{
				Domain:             "example.domain",
				HTTPAddr:           ":8080",
				SMTPAddr:           ":2525",
				AllowExternalRelay: false,
				MaxMessageBytes:    defaultMaxMessageBytes,
			},
			wantCaddy: true,
			caddyContain: []string{
				"example.domain {",
				"reverse_proxy agentpost:8080",
				"www.example.domain {",
				"redir https://example.domain{uri} permanent",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			envFile := filepath.Join(dir, ".env")
			configFile := filepath.Join(dir, "config.yaml")
			caddyFile := filepath.Join(dir, "deploy", "Caddyfile")

			root := repoRoot(t)
			cmd := exec.Command("bash", append([]string{filepath.Join(root, "start.sh")}, tt.args...)...)
			cmd.Env = []string{
				"PATH=" + os.Getenv("PATH"),
				"HOME=" + dir,
				"ENV_FILE=" + envFile,
				"CONFIG_FILE=" + configFile,
				"CADDYFILE=" + caddyFile,
			}
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("start.sh configure failed: %v\n%s", err, output)
			}

			gotEnv := readEnvFile(t, envFile)
			for key, want := range tt.wantEnv {
				if got := gotEnv[key]; got != want {
					t.Fatalf("%s = %q, want %q\nfull env: %#v", key, got, want, gotEnv)
				}
			}
			if _, ok := gotEnv["AGENTPOST_API_TOKEN"]; ok {
				t.Fatalf("configure must not write AGENTPOST_API_TOKEN to .env: %#v", gotEnv)
			}

			var gotConfig Config
			configBytes, err := os.ReadFile(configFile)
			if err != nil {
				t.Fatalf("read generated config: %v", err)
			}
			if err := yaml.Unmarshal(configBytes, &gotConfig); err != nil {
				t.Fatalf("parse generated config: %v\n%s", err, configBytes)
			}
			if gotConfig != tt.wantConfig {
				t.Fatalf("generated config = %+v, want %+v", gotConfig, tt.wantConfig)
			}

			caddyBytes, err := os.ReadFile(caddyFile)
			if !tt.wantCaddy {
				if !os.IsNotExist(err) {
					t.Fatalf("Caddyfile should not be generated, read err = %v, body = %s", err, caddyBytes)
				}
				return
			}
			if err != nil {
				t.Fatalf("read generated Caddyfile: %v", err)
			}
			caddy := string(caddyBytes)
			for _, want := range tt.caddyContain {
				if !strings.Contains(caddy, want) {
					t.Fatalf("generated Caddyfile missing %q:\n%s", want, caddy)
				}
			}
		})
	}
}

func TestDockerArtifactsMatchDeploymentContract(t *testing.T) {
	root := repoRoot(t)
	dockerfileBytes, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	dockerfile := string(dockerfileBytes)
	for _, want := range []string{
		"go build -trimpath -ldflags=\"-s -w\" -o /out/agentpost ./cmd/agentpost",
		"COPY --from=builder /out/agentpost /app/agentpost",
		"ENTRYPOINT [\"/app/agentpost\"]",
		"CMD [\"-config\", \"/app/config.yaml\"]",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Fatalf("Dockerfile missing %q", want)
		}
	}

	composeBytes, err := os.ReadFile(filepath.Join(root, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read docker-compose.yml: %v", err)
	}
	var compose struct {
		Services map[string]struct {
			Build       string         `yaml:"build"`
			Environment map[string]any `yaml:"environment"`
			Volumes     []string       `yaml:"volumes"`
			Healthcheck struct {
				Test []string `yaml:"test"`
			} `yaml:"healthcheck"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(composeBytes, &compose); err != nil {
		t.Fatalf("parse docker-compose.yml: %v", err)
	}
	service, ok := compose.Services["agentpost"]
	if !ok {
		t.Fatalf("docker-compose.yml must define the agentpost service")
	}
	if service.Build != "." {
		t.Fatalf("agentpost build context = %q, want .", service.Build)
	}
	for key, want := range map[string]string{
		"AGENTPOST_HTTP_ADDR":            ":8080",
		"AGENTPOST_ALLOW_EXTERNAL_RELAY": "false",
	} {
		if got := service.Environment[key]; got != want {
			t.Fatalf("agentpost environment %s = %#v, want %q", key, got, want)
		}
	}
	if !containsString(service.Volumes, "./config.yaml:/app/config.yaml:ro") {
		t.Fatalf("agentpost service must mount generated config.yaml read-only: %#v", service.Volumes)
	}
	if got := strings.Join(service.Healthcheck.Test, " "); !strings.Contains(got, "http://127.0.0.1:8080/healthz") {
		t.Fatalf("healthcheck must target the in-container HTTP port, got %#v", service.Healthcheck.Test)
	}

	caddyBytes, err := os.ReadFile(filepath.Join(root, "deploy/Caddyfile"))
	if err != nil {
		t.Fatalf("read deploy/Caddyfile: %v", err)
	}
	if !strings.Contains(string(caddyBytes), "reverse_proxy agentpost:8080") {
		t.Fatalf("Caddyfile must proxy to the docker-compose service name agentpost:\n%s", caddyBytes)
	}
}

func readEnvFile(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	env := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("invalid env line %q", line)
		}
		env[key] = value
	}
	return env
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
