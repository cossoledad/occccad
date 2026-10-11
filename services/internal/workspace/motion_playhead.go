package workspace

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// Exact adoption/conversion uses the same frozen path and solver. Interpolated
// viewport transforms and a neighbouring cached sample are never commit inputs.
func (service *Service) motionPlayhead(parent context.Context, run MotionRun, at float64) (MotionFrame, error) {
	if !finiteMotion(at) || at < 0 || len(run.Frames) == 0 || at > run.Frames[len(run.Frames)-1].TimeSeconds+1e-9 || run.Snapshot.Policy != motionPolicy || run.Snapshot.Digest != motionDigest(run.Snapshot) {
		return MotionFrame{}, fmt.Errorf("%w: invalid qualified playhead", ErrValidation)
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	e, err := newMechanismMotionSolver(ctx, service, run.Snapshot)
	if err != nil {
		return MotionFrame{}, err
	}
	ref := 0.0
	if run.Snapshot.Study.Reference != nil {
		ref, err = motionValue(*run.Snapshot.Study.Reference, true)
		if err != nil {
			return MotionFrame{}, err
		}
	}
	targets, err := motionPathTargets(run.Snapshot.Study)
	if err != nil {
		return MotionFrame{}, err
	}
	var previous *motionPathTarget
	for i := range targets {
		target := targets[i]
		if target.Time > at {
			if previous == nil {
				return MotionFrame{}, fmt.Errorf("%w: missing path initial key", ErrValidation)
			}
			fraction := (at - previous.Time) / (target.Time - previous.Time)
			q := previous.Value + (target.Value-previous.Value)*fraction
			f, err := e.Advance(ctx, ref+q)
			if err != nil {
				return f, err
			}
			f.DriverValue = q
			f.TimeSeconds = at
			return f, nil
		}
		f, err := e.Advance(ctx, ref+target.Value)
		if err != nil {
			return f, err
		}
		if target.Time == at {
			f.DriverValue = target.Value
			f.TimeSeconds = at
			f.KeyframeID = target.KeyframeID
			return f, nil
		}
		previous = &targets[i]
	}
	return MotionFrame{}, fmt.Errorf("%w: playhead outside recording", ErrValidation)
}

func (service *Service) PrepareMotionPose(ctx context.Context, document, actor, base string, req MotionApplyRequest) (CommandRequest, error) {
	view, err := service.GetDocument(ctx, document)
	if err != nil {
		return CommandRequest{}, err
	}
	if view.Product == nil || view.Document.VersionID != base || req.TimeSeconds == nil {
		return CommandRequest{}, fmt.Errorf("%w: invalid motion adoption revision or time", ErrValidation)
	}
	run, err := service.readMotionResult(ctx, document, actor, req.JobID)
	if err != nil {
		return CommandRequest{}, err
	}
	if run.Snapshot.RevisionID != base {
		return CommandRequest{}, fmt.Errorf("%w: historical playback requires conversion revalidation", ErrValidation)
	}
	f, err := service.motionPlayhead(ctx, run, *req.TimeSeconds)
	if err != nil {
		return CommandRequest{}, err
	}
	k := view.Product.Kinematics
	mi := slices.IndexFunc(k.Mechanisms, func(m Mechanism) bool { return m.ID == run.Snapshot.Mechanism.ID })
	if mi < 0 {
		return CommandRequest{}, fmt.Errorf("%w: mechanism removed", ErrValidation)
	}
	k.Mechanisms[mi].Poses = mechanismMemberPoses(k.Mechanisms[mi], f.UnitPoses)
	k.Mechanisms[mi].Coordinates = f.Coordinates
	command := CommandRequest{Type: "SAVE_KINEMATICS", VersionID: base, ActorID: actor, RequestID: newID("motion-adopt"), Kinematics: &k}
	preview, err := service.PreviewCommand(ctx, document, command)
	if err != nil {
		return CommandRequest{}, err
	}
	command.PreviewID = preview.PreviewID
	return command, nil
}
