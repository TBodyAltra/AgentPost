package main

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPagesWorkflowSkipsWhenPagesIsUnavailable(t *testing.T) {
	data, err := os.ReadFile(".github/workflows/pages.yml")
	if err != nil {
		t.Fatalf("read Pages workflow: %v", err)
	}
	workflow := string(data)

	for _, want := range []string{
		"Check GitHub Pages availability",
		`echo "enabled=false" >> "${GITHUB_OUTPUT}"`,
		"This repository does not have GitHub Pages enabled yet.",
		`if: ${{ steps.pages.outputs.enabled == 'true' }}`,
		`if: ${{ needs.build.outputs.pages_enabled == 'true' }}`,
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("Pages workflow missing %q", want)
		}
	}
	if strings.Contains(workflow, "enablement: true") {
		t.Fatalf("configure-pages must not try to create the Pages site with the default GITHUB_TOKEN")
	}

	var parsed struct {
		Jobs map[string]struct {
			Outputs map[string]string `yaml:"outputs"`
			If      string            `yaml:"if"`
			Steps   []struct {
				Name string `yaml:"name"`
				If   string `yaml:"if"`
				Uses string `yaml:"uses"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse Pages workflow YAML: %v", err)
	}

	build := parsed.Jobs["build"]
	if got := build.Outputs["pages_enabled"]; got != "${{ steps.pages.outputs.enabled }}" {
		t.Fatalf("build pages_enabled output = %q", got)
	}
	for _, step := range build.Steps {
		switch step.Name {
		case "Configure Pages", "Upload static site":
			if step.If != "${{ steps.pages.outputs.enabled == 'true' }}" {
				t.Fatalf("%s step if = %q, want enabled gate", step.Name, step.If)
			}
		}
	}

	deploy := parsed.Jobs["deploy"]
	if deploy.If != "${{ needs.build.outputs.pages_enabled == 'true' }}" {
		t.Fatalf("deploy job if = %q, want pages_enabled gate", deploy.If)
	}
}
