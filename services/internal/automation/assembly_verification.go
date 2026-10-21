package automation

import (
	"context"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

type AssemblyVerificationInput = workspace.AutomationAssemblyVerificationInput
type AssemblyVerification = workspace.AutomationAssemblyVerification

type assemblyVerificationReader interface {
	VerifyAutomationAssembly(context.Context, workspace.CommandAuthorizer, string, string, workspace.AutomationAssemblyVerificationInput) (workspace.AutomationAssemblyVerification, error)
}

func (s *Service) VerifyAssembly(ctx context.Context, actor, document string, input AssemblyVerificationInput) (AssemblyVerification, error) {
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return AssemblyVerification{}, err
	}
	reader, ok := s.workspace.(assemblyVerificationReader)
	if !ok {
		return AssemblyVerification{}, fault("ASSEMBLY_VERIFICATION_UNAVAILABLE", "production assembly verification is unavailable")
	}
	release, err := s.acquire(actor)
	if err != nil {
		return AssemblyVerification{}, err
	}
	defer release()
	return reader.VerifyAutomationAssembly(ctx, s.access, actor, document, input)
}
