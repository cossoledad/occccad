package workspace

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
)

func TestFixExplicitPosePreviewCommitAndHistory(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "fixed_pose_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	store, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewWithArtifacts(db, motionWorker(t), artifact.NewService(db, store))
	actor := access.DefaultUserID
	part, err := svc.CreateDocument(t.Context(), CreateDocumentRequest{Type: "PART", Name: "Fix pose source", ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"SPACE", "RELATIVE"} {
		t.Run(mode, func(t *testing.T) {
			view, err := svc.CreateDocument(t.Context(), CreateDocumentRequest{Type: "PRODUCT", Name: "Fix pose", ActorID: actor})
			if err != nil {
				t.Fatal(err)
			}
			apply := func(req CommandRequest) {
				t.Helper()
				req.ActorID = actor
				req.VersionID = view.Document.VersionID
				if req.RequestID == "" {
					req.RequestID = uuid.NewString()
				}
				view, err = svc.ApplyCommand(t.Context(), view.Document.ID, req)
				if err != nil {
					t.Fatal(err)
				}
			}
			apply(CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
			instanceID := view.Product.Instances[0].ID
			pose := InstancePose{Translation: [3]float64{12.34567, 23, 34}, Rotation: [4]float64{0, 0, math.Sqrt(.5), math.Sqrt(.5)}}
			req := CommandRequest{ActorID: actor, RequestID: uuid.NewString(), VersionID: view.Document.VersionID, Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FixMode: mode, FirstAssemblyRef: &AssemblyGeometryRef{InstanceID: instanceID, Kind: "BODY"}, FixedPose: &pose}
			preview, err := svc.PreviewCommand(t.Context(), view.Document.ID, req)
			if err != nil || preview.PreviewID == "" {
				t.Fatal("explicit pose preview", err)
			}
			read, err := svc.GetDocument(t.Context(), view.Document.ID)
			if err != nil || read.Document.VersionID != view.Document.VersionID || len(read.Product.Constraints) != 0 {
				t.Fatal("preview changed Head", err)
			}
			req.PreviewID = preview.PreviewID
			apply(req)
			check := func(want InstancePose) {
				t.Helper()
				c := view.Product.Constraints[0]
				if c.FixedPose == nil || c.FixedPose.Translation != want.Translation || c.FixMode != mode || c.EvaluationStatus != "VERIFIED" {
					t.Fatal("lost fixed definition", c)
				}
				for i, v := range view.Product.Instances[0].Translation {
					if math.Abs(v-want.Translation[i]) > 1e-7 {
						t.Fatal("solver did not adopt explicit pose", view.Product.Instances[0])
					}
				}
				for _, direction := range [][3]float64{{1, 0, 0}, {0, 0, 1}} {
					for _, actual := range []InstancePose{*c.FixedPose, {Rotation: view.Product.Instances[0].Rotation}} {
						if math.Abs(conflictDot(rotateByPose(actual, direction), rotateByPose(want, direction))-1) > 1e-10 {
							t.Fatal("fixed orientation changed", actual)
						}
					}
				}
			}
			check(pose)
			constraintID := view.Product.Constraints[0].ID
			apply(CommandRequest{Type: "UNDO"})
			if len(view.Product.Constraints) != 0 || view.Product.Instances[0].Translation != ([3]float64{}) {
				t.Fatal("Fix creation Undo lost entry pose")
			}
			apply(CommandRequest{Type: "REDO"})
			check(pose)
			changed := pose
			changed.Translation[1] = 45
			changed.Rotation = [4]float64{0, 0, 0, 1}
			apply(CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: constraintID, FixMode: mode, FixedPose: &changed})
			check(changed)
			apply(CommandRequest{Type: "UNDO"})
			check(pose)
		})
	}
}
