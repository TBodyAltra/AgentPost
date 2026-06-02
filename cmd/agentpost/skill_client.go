package main

import (
	_ "embed"
	"strings"
)

//go:embed skills/agentpost-client.md
var embeddedClientAgentSkill string

func appendClientAgentSkill(deploymentSkill string) string {
	client := strings.TrimSpace(embeddedClientAgentSkill)
	if client == "" {
		return deploymentSkill
	}
	deploymentSkill = strings.TrimRight(deploymentSkill, "\n")
	return deploymentSkill + "\n\n---\n\n" + client + "\n"
}
