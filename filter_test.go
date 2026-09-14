package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRewriteRequestReplacesDefaultSystemKeywords(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "string system mentions opencode",
			body: `{"system":"You are OpenCode, an AI coding tool."}`,
			want: "You are Antigravity, an AI coding tool.",
		},
		{
			name: "array system mentions claude code",
			body: `{"system":[{"type":"text","text":"Run as Claude Code."}]}`,
			want: "Run as Antigravity.",
		},
		{
			name: "case insensitive codex",
			body: `{"system":"route this CODEX session"}`,
			want: "route this Antigravity session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rewritten := rewriteRequestBody([]byte(tt.body))
			if !rewritten {
				t.Fatalf("rewritten = false, want true")
			}
			if !containsSystemText(t, got, tt.want) {
				t.Fatalf("rewritten body = %s, want system text %q", got, tt.want)
			}
		})
	}
}

func TestRewriteRequestRewritesInstructionsOnlyWhenEnabled(t *testing.T) {
	defer restoreDefaultFilterConfig(t)

	body := []byte(`{"instructions":"You are Hermes Agent, created by Nous Research.","system":"You are Codex."}`)

	applyFilterConfig(filterConfig{Mode: filterModeRewrite, UseDefaultKeywords: true})
	got, rewritten := rewriteRequestBody(body)
	if !rewritten || strings.Contains(string(got), `"instructions":"You are Antigravity`) {
		t.Fatalf("body = %s, instructions should remain unchanged", got)
	}

	applyFilterConfig(filterConfig{
		Mode:                filterModeRewrite,
		UseDefaultKeywords:  true,
		IncludeInstructions: true,
	})
	got, rewritten = rewriteRequestBodyWithFormat(body, activeFilterConfig(), openAIResponsesFormat)
	if !rewritten {
		t.Fatal("rewritten = false, want true when instructions scanning is enabled")
	}
	if !strings.Contains(string(got), "You are Antigravity, created by Nous Research.") {
		t.Fatalf("body = %s, want Hermes Agent rewritten in instructions", got)
	}
	if !strings.Contains(string(got), `"system":"You are Antigravity."`) {
		t.Fatalf("body = %s, want system rewritten as well", got)
	}
}

func TestInstructionsMappingCanBeConfiguredWithoutDefaultKeywords(t *testing.T) {
	defer restoreDefaultFilterConfig(t)

	applyFilterConfig(filterConfig{
		Mode:                filterModeRewrite,
		IncludeInstructions: true,
		CustomMappings: []rewriteMapping{
			{Match: "Hermes Agent", Replacement: "an intelligent AI coding assistant"},
		},
	})

	got, rewritten := rewriteRequestBodyWithFormat([]byte(`{"instructions":"You are Hermes Agent."}`), activeFilterConfig(), openAIResponsesFormat)
	if !rewritten || !strings.Contains(string(got), "an intelligent AI coding assistant") {
		t.Fatalf("body = %s, rewritten=%v, want custom instructions mapping", got, rewritten)
	}
	if got, rewritten := rewriteRequestBodyWithFormat([]byte(`{"instructions":"You are Codex."}`), activeFilterConfig(), openAIResponsesFormat); rewritten {
		t.Fatalf("body = %s, rewritten=%v, default Codex mapping should be disabled", got, rewritten)
	}
}

func TestInstructionsMatchesNestedFieldsWhenEnabled(t *testing.T) {
	defer restoreDefaultFilterConfig(t)

	applyFilterConfig(filterConfig{
		Mode:                filterModeRewrite,
		IncludeInstructions: true,
		CustomMappings: []rewriteMapping{
			{Match: "Hermes Agent", Replacement: "an intelligent AI coding assistant"},
		},
	})

	body := []byte(`{"instructions":"You are Hermes Agent.","metadata":{"instructions":"Keep Hermes Agent here too."},"tools":[{"parameters":{"instructions":"Keep Hermes Agent here as well."}}]}`)
	got, rewritten := rewriteRequestBodyWithFormat(body, activeFilterConfig(), openAIResponsesFormat)
	if !rewritten {
		t.Fatal("rewritten = false, want instructions fields rewritten")
	}
	text := string(got)
	if !strings.Contains(text, `"instructions":"You are an intelligent AI coding assistant."`) ||
		!strings.Contains(text, `"instructions":"Keep an intelligent AI coding assistant here too."`) ||
		!strings.Contains(text, `"instructions":"Keep an intelligent AI coding assistant here as well."`) {
		t.Fatalf("body = %s, all instructions fields should be rewritten", got)
	}
}

func TestBuiltInKeywordPresetCoversMainstreamCodingToolsAndAgents(t *testing.T) {
	defer restoreDefaultFilterConfig(t)
	applyFilterConfig(defaultFilterConfig())

	for _, mapping := range defaultRewriteMappings {
		keyword := mapping.Match
		t.Run(keyword, func(t *testing.T) {
			decision := classifyRequest([]byte(`{"system":"You are ` + keyword + `."}`))
			if !decision.Blocked {
				t.Fatalf("%q was not detected by built-in preset", keyword)
			}
			got, rewritten := rewriteRequestBody([]byte(`{"system":"You are ` + keyword + `."}`))
			if !rewritten || !strings.Contains(string(got), "You are Antigravity.") {
				t.Fatalf("%q rewrite = %s, changed=%v", keyword, got, rewritten)
			}
		})
	}
}

func TestDefaultFilterModeBlocksAndRewriteMustBeSelected(t *testing.T) {
	cfg, err := parseFilterConfigYAML(nil)
	if err != nil {
		t.Fatalf("parse default config: %v", err)
	}
	if cfg.Mode != filterModeBlock {
		t.Fatalf("default mode = %q, want block", cfg.Mode)
	}

	cfg, err = parseFilterConfigYAML([]byte("mode: rewrite\n"))
	if err != nil {
		t.Fatalf("parse rewrite config: %v", err)
	}
	if cfg.Mode != filterModeRewrite {
		t.Fatalf("configured mode = %q, want rewrite", cfg.Mode)
	}
}

func TestLongerBuiltInNamesAreRewrittenBeforeShortAliases(t *testing.T) {
	got, rewritten := rewriteRequestBody([]byte(`{"system":"Run GitHub Copilot CLI and OpenAI Codex."}`))
	if !rewritten {
		t.Fatal("rewritten = false, want true")
	}
	if string(got) != `{"system":"Run Antigravity and Antigravity."}` {
		t.Fatalf("body = %s, want complete product names replaced once", got)
	}
}

func TestRewriteRequestIgnoresKeywordsOutsideSystem(t *testing.T) {
	body := []byte(`{
		"messages":[{"role":"user","content":"please compare OpenCode and Codex"}],
		"input":"Claude Code is mentioned by the user"
	}`)
	got, rewritten := rewriteRequestBody(body)
	if rewritten {
		t.Fatalf("rewritten = true, want false; body=%s", got)
	}
}

func TestRewriteRequestAllowsCleanInvalidAndStructuralBodies(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "clean json",
			body: `{"system":"You are Antigravity.","messages":[{"role":"user","content":"hello"}]}`,
		},
		{
			name: "invalid json",
			body: `{`,
		},
		{
			name: "empty body",
			body: ``,
		},
		{
			name: "prompt cache key",
			body: `{"prompt_cache_key":"session-cache","system":"plain"}`,
		},
		{
			name: "metadata user id",
			body: `{"metadata":{"user_id":"user-123"},"system":"plain"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rewritten := rewriteRequestBody([]byte(tt.body))
			if rewritten {
				t.Fatalf("rewritten = true, want false; body=%s", got)
			}
		})
	}
}

func containsSystemText(t *testing.T, body []byte, want string) bool {
	t.Helper()

	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		t.Fatalf("decode rewritten body: %v", err)
	}

	found := false
	walkJSON(root, func(path []string, value any) bool {
		if len(path) == 0 || path[len(path)-1] != "system" {
			return true
		}
		found = strings.Contains(collectText(value), want)
		return !found
	})
	return found
}
