package main

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPagesWorkflowPublishesDocsBranch(t *testing.T) {
	data, err := os.ReadFile(".github/workflows/pages.yml")
	if err != nil {
		t.Fatalf("read Pages workflow: %v", err)
	}
	workflow := string(data)

	for _, want := range []string{
		"name: Deploy GitHub Pages",
		"uses: peaceiris/actions-gh-pages@v4",
		"publish_dir: ./docs",
		"publish_branch: gh-pages",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("Pages workflow missing %q", want)
		}
	}

	for _, forbidden := range []string{
		"PAGES_ENABLEMENT_TOKEN",
		"configure-pages",
		"deploy-pages",
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
			Steps []struct {
				Uses string `yaml:"uses"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse Pages workflow YAML: %v", err)
	}

	if parsed.Permissions["contents"] != "write" {
		t.Fatalf("Pages workflow permissions = %#v", parsed.Permissions)
	}

	deploy := parsed.Jobs["deploy"]
	if len(deploy.Steps) < 2 {
		t.Fatalf("deploy job steps = %d, want at least 2", len(deploy.Steps))
	}
}
