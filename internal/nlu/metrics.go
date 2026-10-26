package nlu

import (
	"context"
	"github.com/NIDJEL/assistant-ai/internal/domain"
)

type TurnMetrics = domain.TurnMetrics
type metricsKey struct{}

func WithMetrics(ctx context.Context) (context.Context, *TurnMetrics) {
	m := &TurnMetrics{}
	return context.WithValue(ctx, metricsKey{}, m), m
}
func Metrics(ctx context.Context) *TurnMetrics {
	m, _ := ctx.Value(metricsKey{}).(*TurnMetrics)
	return m
}
