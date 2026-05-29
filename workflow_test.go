package main

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPagesWorkflowUsesGitHubActionsDeployment(t *testing.T) {
	data, err := os.ReadFile(".github/workflows/pages.yml")
	if err != nil {
		t.Fatalf("read Pages workflow: %v", err)
	}
	workflow := string(data)

	for _, want := range []string{
		"name: Deploy GitHub Pages",
		"uses: actions/configure-pages@v5",
		"uses: actions/upload-pages-artifact@v4",
		"uses: actions/deploy-pages@v4",
		"path: docs",
		"name: github-pages",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("Pages workflow missing %q", want)
		}
	}

	for _, forbidden := range []string{
		"PAGES_ENABLEMENT_TOKEN",
		"peaceiris/actions-gh-pages",
		"publish_branch: gh-pages",
		"pages_enabled",
		"enabled=false",
		"Check GitHub Pages availability",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("Pages workflow must not contain %q", forbidden)
		}
	}

	var parsed struct {
		Permissions map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			Environment struct {
				Name string `yaml:"name"`
			} `yaml:"environment"`
			Steps []struct {
				Uses string `yaml:"uses"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse Pages workflow YAML: %v", err)
	}

	if parsed.Permissions["pages"] != "write" || parsed.Permissions["id-token"] != "write" {
		t.Fatalf("Pages workflow permissions = %#v", parsed.Permissions)
	}

	deploy := parsed.Jobs["deploy"]
	if deploy.Environment.Name != "github-pages" {
		t.Fatalf("deploy environment = %q, want github-pages", deploy.Environment.Name)
	}
	if len(deploy.Steps) < 4 {
		t.Fatalf("deploy job steps = %d, want at least 4", len(deploy.Steps))
	}
}
