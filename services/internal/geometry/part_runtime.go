package geometry

import "context"

type PartRuntimeOptions struct {
	ForceCold, ExactOnly bool
	Affinity             string
}
type partRuntimeKey struct{}

func WithPartRuntime(ctx context.Context, options PartRuntimeOptions) context.Context {
	return context.WithValue(ctx, partRuntimeKey{}, options)
}
func PartRuntime(ctx context.Context) PartRuntimeOptions {
	options, _ := ctx.Value(partRuntimeKey{}).(PartRuntimeOptions)
	return options
}
