package setting

import (
	"strings"
)

type ClientIntegration struct {
	Id        string           `json:"id"`
	Name      string           `json:"name"`
	Family    string           `json:"family"`
	Enabled   bool             `json:"enabled"`
	Artifacts []ClientArtifact `json:"artifacts"`
}

type ClientArtifact struct {
	Level             string `json:"level"`
	Kind              string `json:"kind"`
	Platform          string `json:"platform"`
	ContentType       string `json:"contentType"`
	RequiresHelperApp bool   `json:"requiresHelperApp"`
	Label             string `json:"label"`
	Filename          string `json:"filename"`
	Template          string `json:"template"`
}

var clientIntegrations = []ClientIntegration{
	{
		Id:      "claude-code",
		Name:    "Claude Code",
		Family:  "cli",
		Enabled: true,
		Artifacts: []ClientArtifact{
			{
				Level:             "l3",
				Kind:              "deeplink",
				Platform:          "any",
				ContentType:       "text/uri-list",
				RequiresHelperApp: true,
				Label:             "Open with cc-switch",
				Template:          "ccswitch://v1/import?resource=provider&app=claude&endpoint={address}&apiKey={key}&model={model}&homepage={address}&enabled=true",
			},
			{
				Level:             "l1",
				Kind:              "snippet",
				Platform:          "any",
				ContentType:       "text/plain",
				RequiresHelperApp: false,
				Label:             "bash env",
				Filename:          "claude-code.env.sh",
				Template:          "export ANTHROPIC_BASE_URL=\"{address}\"\nexport ANTHROPIC_AUTH_TOKEN=\"{key}\"\nexport ANTHROPIC_MODEL=\"{model}\"\n",
			},
			{
				Level:             "l1",
				Kind:              "snippet",
				Platform:          "windows",
				ContentType:       "text/plain",
				RequiresHelperApp: false,
				Label:             "PowerShell env",
				Filename:          "claude-code.env.ps1",
				Template:          "$env:ANTHROPIC_BASE_URL = \"{address}\"\n$env:ANTHROPIC_AUTH_TOKEN = \"{key}\"\n$env:ANTHROPIC_MODEL = \"{model}\"\n",
			},
			{
				Level:             "l1",
				Kind:              "snippet",
				Platform:          "any",
				ContentType:       "application/json",
				RequiresHelperApp: false,
				Label:             "Claude settings.json",
				Filename:          "settings.json",
				Template:          "{\n  \"env\": {\n    \"ANTHROPIC_BASE_URL\": \"{address}\",\n    \"ANTHROPIC_AUTH_TOKEN\": \"{key}\",\n    \"ANTHROPIC_MODEL\": \"{model}\"\n  }\n}\n",
			},
			{
				Level:             "l2",
				Kind:              "script",
				Platform:          "any",
				ContentType:       "text/x-shellscript",
				RequiresHelperApp: false,
				Label:             "Install script",
				Filename:          "configure-claude-code.sh",
				Template:          "#!/usr/bin/env bash\nset -euo pipefail\nmkdir -p \"${HOME}/.claude\"\ncat > \"${HOME}/.claude/settings.json\" <<'EOF'\n{\n  \"env\": {\n    \"ANTHROPIC_BASE_URL\": \"{address}\",\n    \"ANTHROPIC_AUTH_TOKEN\": \"{key}\",\n    \"ANTHROPIC_MODEL\": \"{model}\"\n  }\n}\nEOF\n",
			},
			{
				Level:             "l2",
				Kind:              "script",
				Platform:          "windows",
				ContentType:       "text/plain",
				RequiresHelperApp: false,
				Label:             "Install script (PowerShell)",
				Filename:          "configure-claude-code.ps1",
				Template:          "New-Item -ItemType Directory -Force -Path (Join-Path $HOME '.claude') | Out-Null\n@'\n{\n  \"env\": {\n    \"ANTHROPIC_BASE_URL\": \"{address}\",\n    \"ANTHROPIC_AUTH_TOKEN\": \"{key}\",\n    \"ANTHROPIC_MODEL\": \"{model}\"\n  }\n}\n'@ | Set-Content -Encoding utf8 -Path (Join-Path (Join-Path $HOME '.claude') 'settings.json')\n",
			},
		},
	},
	{
		Id:      "claude-code-extension",
		Name:    "Claude Code Extension",
		Family:  "editor",
		Enabled: true,
		Artifacts: []ClientArtifact{
			{
				Level:             "l1",
				Kind:              "snippet",
				Platform:          "any",
				ContentType:       "text/plain",
				RequiresHelperApp: false,
				Label:             "Claude env",
				Filename:          "claude-code-extension.env.sh",
				Template:          "export ANTHROPIC_BASE_URL=\"{address}\"\nexport ANTHROPIC_AUTH_TOKEN=\"{key}\"\nexport ANTHROPIC_MODEL=\"{model}\"\n",
			},
			{
				Level:             "l1",
				Kind:              "steps",
				Platform:          "any",
				ContentType:       "text/markdown",
				RequiresHelperApp: false,
				Label:             "Manual setup steps",
				Template:          "1. Open the extension settings.\n2. Set the provider to an OpenAI-compatible endpoint.\n3. Set the base URL to {address}/v1.\n4. Paste the API key {key}.\n5. Select model {model}.\n",
			},
		},
	},
	{
		Id:      "codex",
		Name:    "OpenAI Codex",
		Family:  "cli",
		Enabled: true,
		Artifacts: []ClientArtifact{
			{
				Level:             "l3",
				Kind:              "deeplink",
				Platform:          "any",
				ContentType:       "text/uri-list",
				RequiresHelperApp: true,
				Label:             "Open with cc-switch",
				Template:          "ccswitch://v1/import?resource=provider&app=codex&endpoint={address}/v1&apiKey={key}&model={model}&homepage={address}&enabled=true",
			},
			{
				Level:             "l1",
				Kind:              "snippet",
				Platform:          "any",
				ContentType:       "application/toml",
				RequiresHelperApp: false,
				Label:             "config.toml",
				Filename:          "config.toml",
				Template:          "[model_providers.lemonhub]\nname = \"LemonHub\"\nbase_url = \"{address}/v1\"\nwire_api = \"chat\"\n\n[profiles.lemonhub]\nmodel_provider = \"lemonhub\"\nmodel = \"{model}\"\n",
			},
			{
				Level:             "l1",
				Kind:              "snippet",
				Platform:          "any",
				ContentType:       "application/json",
				RequiresHelperApp: false,
				Label:             "auth.json",
				Filename:          "auth.json",
				Template:          "{\n  \"OPENAI_API_KEY\": \"{key}\"\n}\n",
			},
			{
				Level:             "l2",
				Kind:              "script",
				Platform:          "any",
				ContentType:       "text/x-shellscript",
				RequiresHelperApp: false,
				Label:             "Install script",
				Filename:          "configure-codex.sh",
				Template:          "#!/usr/bin/env bash\nset -euo pipefail\nmkdir -p \"${HOME}/.codex\"\ncat > \"${HOME}/.codex/config.toml\" <<'EOF'\n[model_providers.lemonhub]\nname = \"LemonHub\"\nbase_url = \"{address}/v1\"\nwire_api = \"chat\"\n\n[profiles.lemonhub]\nmodel_provider = \"lemonhub\"\nmodel = \"{model}\"\nEOF\ncat > \"${HOME}/.codex/auth.json\" <<'EOF'\n{\n  \"OPENAI_API_KEY\": \"{key}\"\n}\nEOF\n",
			},
			{
				Level:             "l2",
				Kind:              "script",
				Platform:          "windows",
				ContentType:       "text/plain",
				RequiresHelperApp: false,
				Label:             "Install script (PowerShell)",
				Filename:          "configure-codex.ps1",
				Template:          "New-Item -ItemType Directory -Force -Path (Join-Path $HOME '.codex') | Out-Null\n@'\n[model_providers.lemonhub]\nname = \"LemonHub\"\nbase_url = \"{address}/v1\"\nwire_api = \"chat\"\n\n[profiles.lemonhub]\nmodel_provider = \"lemonhub\"\nmodel = \"{model}\"\n'@ | Set-Content -Encoding utf8 -Path (Join-Path (Join-Path $HOME '.codex') 'config.toml')\n@'\n{\n  \"OPENAI_API_KEY\": \"{key}\"\n}\n'@ | Set-Content -Encoding utf8 -Path (Join-Path (Join-Path $HOME '.codex') 'auth.json')\n",
			},
		},
	},
	{
		Id:      "gemini",
		Name:    "Gemini CLI",
		Family:  "cli",
		Enabled: true,
		Artifacts: []ClientArtifact{
			{
				Level:             "l3",
				Kind:              "deeplink",
				Platform:          "any",
				ContentType:       "text/uri-list",
				RequiresHelperApp: true,
				Label:             "Open with cc-switch",
				Template:          "ccswitch://v1/import?resource=provider&app=gemini&endpoint={address}&apiKey={key}&model={model}&homepage={address}&enabled=true",
			},
			{
				Level:             "l1",
				Kind:              "snippet",
				Platform:          "any",
				ContentType:       "text/plain",
				RequiresHelperApp: false,
				Label:             "Environment variables",
				Filename:          "gemini.env.sh",
				Template:          "# Best-effort: verify your Gemini CLI build accepts these variables.\nexport GOOGLE_GEMINI_BASE_URL=\"{address}\"\nexport GOOGLE_GEMINI_API_KEY=\"{key}\"\nexport GOOGLE_GEMINI_MODEL=\"{model}\"\n",
			},
			{
				Level:             "l2",
				Kind:              "script",
				Platform:          "any",
				ContentType:       "text/x-shellscript",
				RequiresHelperApp: false,
				Label:             "Install script",
				Filename:          "configure-gemini.sh",
				Template:          "#!/usr/bin/env bash\nset -euo pipefail\nexport GOOGLE_GEMINI_BASE_URL=\"{address}\"\nexport GOOGLE_GEMINI_API_KEY=\"{key}\"\nexport GOOGLE_GEMINI_MODEL=\"{model}\"\necho \"Configured Gemini CLI environment for {model}.\"\n",
			},
			{
				Level:             "l2",
				Kind:              "script",
				Platform:          "windows",
				ContentType:       "text/plain",
				RequiresHelperApp: false,
				Label:             "Install script (PowerShell)",
				Filename:          "configure-gemini.ps1",
				Template:          "[Environment]::SetEnvironmentVariable('GOOGLE_GEMINI_BASE_URL', '{address}', 'User')\n[Environment]::SetEnvironmentVariable('GOOGLE_GEMINI_API_KEY', '{key}', 'User')\n[Environment]::SetEnvironmentVariable('GOOGLE_GEMINI_MODEL', '{model}', 'User')\nWrite-Host 'Updated user environment variables. Restart your shell to use the new Gemini CLI settings.'\n",
			},
		},
	},
	{
		Id:      "chatbox",
		Name:    "Chatbox",
		Family:  "desktop",
		Enabled: true,
		Artifacts: []ClientArtifact{
			{
				Level:             "l3",
				Kind:              "deeplink",
				Platform:          "any",
				ContentType:       "text/uri-list",
				RequiresHelperApp: false,
				Label:             "Open Chatbox import",
				Template:          "chatbox://provider/import?config={chatboxConfig}",
			},
			{
				Level:             "l1",
				Kind:              "steps",
				Platform:          "any",
				ContentType:       "text/markdown",
				RequiresHelperApp: false,
				Label:             "Manual setup steps",
				Template:          "1. Open Chatbox provider settings.\n2. Add an OpenAI-compatible provider.\n3. Set the API host to {address}.\n4. Paste the API key {key}.\n5. Use model {model}.\n",
			},
		},
	},
	{
		Id:      "cherry-studio",
		Name:    "Cherry Studio",
		Family:  "desktop",
		Enabled: true,
		Artifacts: []ClientArtifact{
			{
				Level:             "l3",
				Kind:              "deeplink",
				Platform:          "any",
				ContentType:       "text/uri-list",
				RequiresHelperApp: false,
				Label:             "Open Cherry Studio import",
				Template:          "cherrystudio://providers/api-keys?v=1&data={cherryConfig}",
			},
			{
				Level:             "l1",
				Kind:              "steps",
				Platform:          "any",
				ContentType:       "text/markdown",
				RequiresHelperApp: false,
				Label:             "Manual setup steps",
				Template:          "1. Open Cherry Studio provider settings.\n2. Add an OpenAI-compatible provider.\n3. Set the base URL to {address}/v1.\n4. Paste the API key {key}.\n5. Use model {model}.\n",
			},
		},
	},
	{
		Id:      "cline",
		Name:    "Cline",
		Family:  "editor",
		Enabled: true,
		Artifacts: []ClientArtifact{
			{
				Level:             "l1",
				Kind:              "snippet",
				Platform:          "any",
				ContentType:       "application/json",
				RequiresHelperApp: false,
				Label:             "settings.json",
				Filename:          "cline-settings.json",
				Template:          "{\n  \"openai.apiBase\": \"{address}/v1\",\n  \"openai.apiKey\": \"{key}\",\n  \"openai.model\": \"{model}\"\n}\n",
			},
			{
				Level:             "l1",
				Kind:              "steps",
				Platform:          "any",
				ContentType:       "text/markdown",
				RequiresHelperApp: false,
				Label:             "Manual setup steps",
				Template:          "1. Open Cline settings.\n2. Select OpenAI-compatible provider.\n3. Set base URL to {address}/v1.\n4. Set the API key to {key}.\n5. Choose model {model}.\n",
			},
		},
	},
	{
		Id:      "roo",
		Name:    "Roo",
		Family:  "editor",
		Enabled: true,
		Artifacts: []ClientArtifact{
			{
				Level:             "l1",
				Kind:              "snippet",
				Platform:          "any",
				ContentType:       "application/json",
				RequiresHelperApp: false,
				Label:             "settings.json",
				Filename:          "roo-settings.json",
				Template:          "{\n  \"openai.apiBase\": \"{address}/v1\",\n  \"openai.apiKey\": \"{key}\",\n  \"openai.model\": \"{model}\"\n}\n",
			},
			{
				Level:             "l1",
				Kind:              "steps",
				Platform:          "any",
				ContentType:       "text/markdown",
				RequiresHelperApp: false,
				Label:             "Manual setup steps",
				Template:          "1. Open Roo settings.\n2. Select OpenAI-compatible provider.\n3. Set base URL to {address}/v1.\n4. Set the API key to {key}.\n5. Choose model {model}.\n",
			},
		},
	},
	{
		Id:      "continue",
		Name:    "Continue",
		Family:  "editor",
		Enabled: true,
		Artifacts: []ClientArtifact{
			{
				Level:             "l1",
				Kind:              "snippet",
				Platform:          "any",
				ContentType:       "application/yaml",
				RequiresHelperApp: false,
				Label:             "config.yaml",
				Filename:          "continue-config.yaml",
				Template:          "models:\n  - name: lemonhub\n    provider: openai\n    apiBase: {address}/v1\n    apiKey: {key}\n    model: {model}\n",
			},
			{
				Level:             "l1",
				Kind:              "steps",
				Platform:          "any",
				ContentType:       "text/markdown",
				RequiresHelperApp: false,
				Label:             "Manual setup steps",
				Template:          "1. Open Continue configuration.\n2. Add an OpenAI provider.\n3. Set apiBase to {address}/v1.\n4. Set apiKey to {key}.\n5. Select model {model}.\n",
			},
		},
	},
}

func GetClientIntegrations() []ClientIntegration {
	out := make([]ClientIntegration, 0, len(clientIntegrations))
	for _, item := range clientIntegrations {
		cloned := item
		cloned.Artifacts = append([]ClientArtifact(nil), item.Artifacts...)
		out = append(out, cloned)
	}
	return out
}

func ResolveClientIntegration(tool, level, platform string) (ClientIntegration, ClientArtifact, bool) {
	tool = normalizeClientIntegrationKey(tool)
	level = normalizeClientIntegrationKey(level)
	platform = normalizeClientIntegrationKey(platform)

	integrations := GetClientIntegrations()
	for _, integration := range integrations {
		if normalizeClientIntegrationKey(integration.Id) != tool {
			continue
		}
		if artifact, ok := resolveArtifact(integration.Artifacts, level, platform); ok {
			return integration, artifact, true
		}
		return integration, ClientArtifact{}, false
	}
	return ClientIntegration{}, ClientArtifact{}, false
}

func resolveArtifact(artifacts []ClientArtifact, level, platform string) (ClientArtifact, bool) {
	if len(artifacts) == 0 {
		return ClientArtifact{}, false
	}
	for _, candidateLevel := range levelFallbackOrder(level) {
		if artifact, ok := findArtifact(artifacts, candidateLevel, platform); ok {
			return artifact, true
		}
		if artifact, ok := findArtifact(artifacts, candidateLevel, ""); ok {
			return artifact, true
		}
	}
	if artifact, ok := findArtifact(artifacts, "", platform); ok {
		return artifact, true
	}
	if artifact, ok := findArtifact(artifacts, "", ""); ok {
		return artifact, true
	}
	return artifacts[0], true
}

func findArtifact(artifacts []ClientArtifact, level, platform string) (ClientArtifact, bool) {
	for _, artifact := range artifacts {
		if level != "" && normalizeClientIntegrationKey(artifact.Level) != level {
			continue
		}
		if platform != "" && normalizeClientIntegrationKey(artifact.Platform) != platform {
			continue
		}
		return artifact, true
	}
	return ClientArtifact{}, false
}

func levelFallbackOrder(level string) []string {
	switch normalizeClientIntegrationKey(level) {
	case "l3":
		return []string{"l3", "l2", "l1", "l0"}
	case "l2":
		return []string{"l2", "l1", "l0"}
	case "l1":
		return []string{"l1", "l0"}
	case "l0":
		return []string{"l0"}
	default:
		return []string{"l3", "l2", "l1", "l0"}
	}
}

func normalizeClientIntegrationKey(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, " ", "-")))
}
