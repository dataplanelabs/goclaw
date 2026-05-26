package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/skills"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// UseSkillTool activates a skill: resolves slug → SKILL.md content + structured
// paths, optionally validating that the calling agent has it granted. Replaces
// the prior tracing-marker behavior where the LLM had to discover paths itself.
type UseSkillTool struct {
	loader      *skills.Loader
	skillAccess store.SkillAccessStore
}

func NewUseSkillTool(loader *skills.Loader) *UseSkillTool {
	return &UseSkillTool{loader: loader}
}

// SetSkillAccessStore enables per-agent grant validation for managed skills.
func (t *UseSkillTool) SetSkillAccessStore(sas store.SkillAccessStore) {
	t.skillAccess = sas
}

func (t *UseSkillTool) Name() string { return "use_skill" }

func (t *UseSkillTool) Description() string {
	return "Activate a skill by name or slug. Returns the SKILL.md content inline plus structured paths to bundled assets and scripts. Call this once before working on a task the skill describes — the response contains everything you need to proceed."
}

func (t *UseSkillTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Skill name or slug to activate",
			},
		},
		"required": []string{"name"},
	}
}

func (t *UseSkillTool) Execute(ctx context.Context, args map[string]any) *Result {
	name, _ := args["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrorResult("name parameter is required")
	}
	if strings.ContainsAny(name, "/\\") || strings.HasPrefix(name, ".") {
		return ErrorResult(fmt.Sprintf("invalid skill name: %q", name))
	}

	payload, err := t.loader.LoadActivationPayload(ctx, name)
	if err != nil {
		return ErrorResult(err.Error())
	}

	if err := t.checkGrant(ctx, payload); err != nil {
		return ErrorResult(err.Error())
	}

	out, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return ErrorResult(fmt.Sprintf("marshal activation payload: %v", marshalErr))
	}

	slog.Info("skill.activated",
		"skill", payload.Slug,
		"source", payload.Source,
		"base_dir", payload.BaseDir,
		"assets", len(payload.AssetPaths),
		"scripts", len(payload.ScriptPaths),
		"inline_md", payload.SkillMDContent != "",
	)
	return NewResult(string(out))
}

// checkGrant enforces per-agent access for managed skills. Filesystem-only
// skills (source != "managed") are always allowed — same convention as
// skill_search.filterByAccess.
func (t *UseSkillTool) checkGrant(ctx context.Context, payload *skills.ActivationPayload) error {
	if t.skillAccess == nil || payload.Source != "managed" {
		return nil
	}
	agentID := store.AgentIDFromContext(ctx)
	if agentID == uuid.Nil {
		return nil
	}
	accessible, listErr := t.skillAccess.ListAccessible(ctx, agentID, store.UserIDFromContext(ctx))
	if listErr != nil {
		// Fail closed on activation — search-time fails open because it's read-only listing,
		// but activation is the gate that opens skill assets to the agent.
		slog.Warn("use_skill: grant check DB error, denying", "skill", payload.Slug, "error", listErr)
		return fmt.Errorf("skill_grant_check_failed: transient error checking grant for %q, retry", payload.Slug)
	}
	for _, s := range accessible {
		if s.Slug == payload.Slug {
			return nil
		}
	}
	return fmt.Errorf("skill_not_granted: %q is not granted to this agent. Ask an admin to grant it via gcplane", payload.Slug)
}
