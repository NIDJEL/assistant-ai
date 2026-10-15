package nlp

import "context"

type pendingExpectationKey struct{}

// Only the dialogue slot name crosses the semantic boundary.
func WithPendingExpectation(ctx context.Context, expectation string) context.Context {
	return context.WithValue(ctx, pendingExpectationKey{}, expectation)
}

func PendingExpectation(ctx context.Context) string {
	value, _ := ctx.Value(pendingExpectationKey{}).(string)
	return value
}
