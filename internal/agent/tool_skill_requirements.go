package agent

import (
	"context"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/skills"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

func (l *Loop) requiredSkillForTool(toolName string) string {
	if l == nil || len(l.toolSkillRequirements) == 0 {
		return ""
	}
	return l.toolSkillRequirements[toolName]
}

func (l *Loop) enforceToolSkillRequirement(ctx context.Context, toolName string) *tools.Result {
	required := l.requiredSkillForTool(toolName)
	if required == "" {
		return nil
	}
	if sc := skills.SkillContextFromContext(ctx); sc != nil && sc.IsActivated(required) {
		return nil
	}
	return tools.ErrorResult(fmt.Sprintf(
		`tool_skill_required: call use_skill with name %q before %s, then retry %s using the skill instructions.`,
		required, toolName, toolName,
	))
}
