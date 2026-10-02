package workspace

import (
	"context"
	"time"
)

// Keep the existing numerical limit, but reserve the parent's final portion
// for recording failure and returning a definition candidate. Never detach.
func assemblyNumericalContext(parent context.Context) (context.Context, context.CancelFunc) {
	budget := 10 * time.Second
	if deadline, ok := parent.Deadline(); ok {
		remaining := time.Until(deadline)
		reserve := remaining / 4
		if reserve > 250*time.Millisecond {
			reserve = 250 * time.Millisecond
		}
		if available := remaining - reserve; available < budget {
			budget = available
		}
	}
	return context.WithTimeout(parent, budget)
}
