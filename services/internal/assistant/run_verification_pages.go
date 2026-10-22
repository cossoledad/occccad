package assistant

import (
	"context"

	"github.com/occccad/occccad/internal/automation"
)

// Existing per-request check/byte boundaries must not become a total goal
// requirement limit. Every page uses the same explicit immutable Revision.
func (s *Service) verifyPartGoalChecks(ctx context.Context, r Run, input automation.PartVerificationRequest) ([]automation.PartCheckResult, error) {
	remaining := input.Checks
	out := []automation.PartCheckResult{}
	for len(remaining) > 0 {
		count := min(len(remaining), automation.MaxPartVerificationChecks)
		input.Checks = remaining[:count]
		for count > 1 && len(marshalJSON(input)) > automation.MaxPartVerificationBytes {
			count--
			input.Checks = remaining[:count]
		}
		result, err := s.automation.VerifyPart(ctx, r.ActorID, r.DocumentID, input)
		if err != nil {
			return nil, err
		}
		out = append(out, result.Checks...)
		remaining = remaining[count:]
	}
	return out, nil
}
