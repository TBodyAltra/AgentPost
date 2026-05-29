package main

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPagesWorkflowDeploysDocsSite(t *testing.T) {
	data, err := os.ReadFile(".github/workflows/pages.yml")
	if err != nil {
		t.Fatalf("read Pages workflow: %v", err)
	}
	workflow := string(data)

	for _, want := range []string{
		"name: Deploy GitHub Pages",
		"uses: actions/configure-pages@v6",
		"enablement: true",
		"uses: actions/upload-pages-artifact@v5",
		"uses: actions/deploy-pages@v5",
		"path: docs",
		"name: github-pages",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("Pages workflow missing %q", want)
		}
	}

	for _, forbidden := range []string{
		"PAGES_ENABLEMENT_TOKEN",
		"Check GitHub Pages availability",
		"pages_enabled",
		"enabled=false",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("Pages workflow must not contain %q", forbidden)
		}
	}

	var parsed struct {
		Permissions map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			If    string `yaml:"if"`
			Steps []struct {
				Name string `yaml:"name"`
				If   string `yaml:"if"`
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

	build := parsed.Jobs["build"]
	for _, step := range build.Steps {
		if step.If != "" {
			t.Fatalf("build step %q must not be gated: if=%q", step.Name, step.If)
		}
	}

	deploy := parsed.Jobs["deploy"]
	if deploy.If != "" {
		t.Fatalf("deploy job must not be gated: if=%q", deploy.If)
	}
}
