package nlp

import (
	"context"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

type LLM interface {
	Interpret(context.Context, string, time.Time) (domain.Command, error)
}
type Hybrid struct {
	Parser   Parser
	Fallback LLM
}

func (h Hybrid) Interpret(ctx context.Context, text string, now time.Time) (domain.Command, error) {
	base, err := h.Parser.Interpret(ctx, text, now)
	if err != nil {
		return base, err
	}
	if base.Confidence >= .8 || h.Fallback == nil {
		return base, nil
	}
	cmd, err := h.Fallback.Interpret(ctx, text, now)
	if ctx.Err() != nil {
		return base, ctx.Err()
	}
	if err != nil {
		base.Provider = "rules_fallback"
		return base, nil
	}
	if err = cmd.Validate(); err != nil {
		base.Provider = "rules_fallback"
		return base, nil
	}
	if base.Date != nil {
		cmd.Date = base.Date
	}
	if base.Time != nil {
		cmd.Time = base.Time
	}
	cmd.Provider = "ollama"
	return cmd, nil
}
