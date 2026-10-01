package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"strings"
	"testing"
)

func boolPointer(value bool) *bool { return &value }

func TestAssemblyAngleShortcutPreviewCommitThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "cylinder-r6-h12")
	support := f.support(t, part, "PLANE")
	for _, relation := range []string{"PARALLEL", "PERPENDICULAR"} {
		t.Run(relation, func(t *testing.T) {
			p, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Angle shortcut candidate"})
			if err != nil {
				t.Fatal(err)
			}
			id := p.Document.ID
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				r.ActorID = f.actor
				p, err = f.service.ApplyCommand(t.Context(), id, r)
				if err != nil {
					t.Fatal(err)
				}
			}
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
			a, b := support, support
			a.InstanceID = p.Product.Instances[0].ID
			b.InstanceID = p.Product.Instances[1].ID
			apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: b.InstanceID, Translation: [3]float64{17, 3, 5}, Rotation: [4]float64{0, math.Sin(.2), 0, math.Cos(.2)}})
			apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: a.InstanceID, Kind: "BODY"}})
			head := p.Document.VersionID
			mode := "DRIVING"
			r := workspace.CommandRequest{ActorID: f.actor, RequestID: id + "-shortcut", Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "ANGLE", ConstraintFamily: "Angle", AngleRelation: relation, FirstAssemblyRef: &a, SecondAssemblyRef: &b, DirectionRelation: "SAME", ConstraintMode: &mode}
			invalid := r
			empty := ""
			invalid.QuantityExpression = &empty
			_, err = f.service.PreviewCommand(t.Context(), id, invalid)
			if err == nil || !strings.Contains(err.Error(), "relation has no editable Quantity parameter") {
				t.Fatal("reproduction/strict Quantity rejection", err)
			}
			preview, err := f.service.PreviewCommand(t.Context(), id, r)
			if err != nil || preview.PreviewID == "" {
				t.Fatal("shortcut preview", err, preview)
			}
			read, err := f.service.GetDocument(t.Context(), id)
			if err != nil || read.Document.VersionID != head {
				t.Fatal("Preview moved Head", err)
			}
			r.PreviewID = preview.PreviewID
			apply(r)
			c := p.Product.Constraints[1]
			if c.AngleRelation != relation || c.QuantityParameter != nil || c.EvaluationStatus != "VERIFIED" {
				t.Fatal("adopted shortcut", c)
			}
			assertFourFamilyGeometry(t, f, id, p, fourFamilyCase{family: "Angle", first: "PLANE", second: "PLANE", relation: relation}, a, b, 0)
			retry, err := f.service.ApplyCommand(t.Context(), id, r)
			if err != nil || retry.Document.VersionID != p.Document.VersionID {
				t.Fatal("shortcut idempotent retry", err)
			}
			// Shared edit path switches relation without injecting stale expressions.
			next := "PERPENDICULAR"
			if relation == next {
				next = "PARALLEL"
			}
			r = workspace.CommandRequest{ActorID: f.actor, RequestID: id + "-edit-shortcut", Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: c.ID, ConstraintFamily: "Angle", AngleRelation: next, FirstAssemblyRef: &a, SecondAssemblyRef: &b, DirectionRelation: "SAME", ConstraintMode: &mode}
			preview, err = f.service.PreviewCommand(t.Context(), id, r)
			if err != nil || preview.PreviewID == "" {
				t.Fatal(err, preview)
			}
			r.PreviewID = preview.PreviewID
			apply(r)
			assertFourFamilyGeometry(t, f, id, p, fourFamilyCase{family: "Angle", first: "PLANE", second: "PLANE", relation: next}, a, b, 0)
			if p.Product.Constraints[1].AngleRelation != next || p.Product.Constraints[1].QuantityParameter != nil {
				t.Fatal("shortcut edit definition")
			}
		})
	}
}

func TestAssemblyEditCandidateIntentThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "cylinder-r6-h12")
	support := f.support(t, part, "PLANE")
	product, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Candidate edit regression"})
	if err != nil {
		t.Fatal(err)
	}
	id := product.Document.ID
	serial := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		serial++
		r.ActorID = f.actor
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-%d", id, serial)
		}
		product, err = f.service.ApplyCommand(t.Context(), id, r)
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 4; i++ {
		apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	}
	refs := []workspace.AssemblyGeometryRef{}
	for i, instance := range product.Product.Instances {
		r := support
		r.InstanceID = instance.ID
		refs = append(refs, r)
		apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: instance.ID, Translation: [3]float64{float64(i * 17), float64(i * 3), 5}, Rotation: [4]float64{0, math.Sin(.15 * float64(i)), 0, math.Cos(.15 * float64(i))}})
	}
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT", ConstraintFamily: "Coincidence", FirstAssemblyRef: &refs[0], SecondAssemblyRef: &refs[1], DirectionRelation: "OPPOSITE", DistanceRelation: "UNSIGNED"})
	cid := product.Product.Constraints[0].ID
	for i, direction := range []string{"SAME", "OPPOSITE", "UNDEFINED", "SAME", "OPPOSITE", "SAME"} {
		t.Run(fmt.Sprintf("direction-%d-%s", i, direction), func(t *testing.T) {
			first, second := refs[0], refs[1]
			if i >= 3 {
				first = refs[2]
			}
			if i >= 4 {
				second = refs[3]
			}
			if i == 5 {
				first, second = refs[1], refs[2]
			}
			r := workspace.CommandRequest{ActorID: f.actor, RequestID: fmt.Sprintf("%s-edit-%d", id, i), Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: cid, ConstraintFamily: "Coincidence", FirstAssemblyRef: &first, SecondAssemblyRef: &second, DirectionRelation: direction, DistanceRelation: "UNSIGNED", InteractionID: "editor", PreviewSequence: uint64(i + 1)}
			head := product.Document.VersionID
			preview, e := f.service.PreviewCommand(t.Context(), id, r)
			if e != nil || preview.PreviewID == "" {
				t.Fatal("preview", e, preview)
			}
			read, e := f.service.GetDocument(t.Context(), id)
			if e != nil || read.Document.VersionID != head {
				t.Fatal("Preview advanced Head", e)
			}
			r.PreviewID = preview.PreviewID
			if i == 0 {
				// Exact old UI split: the hidden UNSIGNED field was omitted by
				// validateFields, even though the visible SAME direction was identical.
				wrong := r
				wrong.DistanceRelation = ""
				_, e = f.service.ApplyCommand(t.Context(), id, wrong)
				if e == nil || !strings.Contains(e.Error(), "intent_digest_mismatch") || !strings.Contains(e.Error(), "$.distanceRelation") {
					t.Fatal("hidden-field repro", e)
				}
				t.Log("confirmed original hidden-field mismatch:", e)
			}
			product, e = f.service.ApplyCommand(t.Context(), id, r)
			if e != nil {
				t.Fatal("same normalized intent commit", e)
			}
			c := product.Product.Constraints[0]
			if c.DirectionRelation != direction || c.First.InstanceID != first.InstanceID || c.Second.InstanceID != second.InstanceID || c.EvaluationStatus != "VERIFIED" {
				t.Fatal("wrong adopted definition", c)
			}
			assertFourFamilyGeometry(t, f, id, product, fourFamilyCase{family: "Coincidence", first: "PLANE", second: "PLANE"}, first, second, 0)
			inspected, e := f.service.InspectAssemblySupports(t.Context(), id, []workspace.AssemblyGeometryRef{c.First, *c.Second})
			if e != nil {
				t.Fatal(e)
			}
			poses := map[string]workspace.ProductInstance{}
			for _, p := range product.Product.Instances {
				poses[p.ID] = p
			}
			u := compositionRotate(poses[first.InstanceID].Rotation, inspected.Supports[0].Descriptor.Direction)
			v := compositionRotate(poses[second.InstanceID].Rotation, inspected.Supports[1].Descriptor.Direction)
			dot := u[0]*v[0] + u[1]*v[1] + u[2]*v[2]
			if direction == "SAME" && dot < 1-1e-8 || direction == "OPPOSITE" && dot > -1+1e-8 {
				t.Fatal("actual normal direction", direction, dot)
			}
			retry, e := f.service.ApplyCommand(t.Context(), id, r)
			if e != nil || retry.Document.VersionID != product.Document.VersionID {
				t.Fatal("successful retry was not idempotent", e)
			}
		})
	}
	t.Run("real-stale-base-and-cancel", func(t *testing.T) {
		c := product.Product.Constraints[0]
		r := workspace.CommandRequest{ActorID: f.actor, RequestID: id + "-stale-edit", Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: cid, ConstraintFamily: c.Family, FirstAssemblyRef: &c.First, SecondAssemblyRef: c.Second, DirectionRelation: "OPPOSITE", DistanceRelation: "UNSIGNED"}
		p, e := f.service.PreviewCommand(t.Context(), id, r)
		if e != nil || p.PreviewID == "" {
			t.Fatal(e, p)
		}
		apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{cid}, Suppressed: boolPointer(true)})
		r.PreviewID = p.PreviewID
		head := product.Document.VersionID
		_, e = f.service.ApplyCommand(t.Context(), id, r)
		if e == nil || !strings.Contains(e.Error(), "base_revision_mismatch") {
			t.Fatal("real changed baseline not rejected", e)
		}
		read, e := f.service.GetDocument(t.Context(), id)
		if e != nil || read.Document.VersionID != head {
			t.Fatal("failed candidate committed", e)
		}
		apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{cid}, Suppressed: boolPointer(false)})
		r.RequestID = id + "-cancelled-edit"
		r.PreviewID = ""
		p, e = f.service.PreviewCommand(t.Context(), id, r)
		if e != nil || p.PreviewID == "" {
			t.Fatal(e, p)
		}
		f.service.DiscardPreview(id, f.actor, p.PreviewID)
		r.PreviewID = p.PreviewID
		_, e = f.service.ApplyCommand(t.Context(), id, r)
		if e == nil || !strings.Contains(e.Error(), "candidate_missing_or_consumed") {
			t.Fatal("cancelled candidate adopted", e)
		}
	})
	t.Run("nested-accepted-display-scopes", func(t *testing.T) {
		parent, e := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Repeated owning Product"})
		if e != nil {
			t.Fatal(e)
		}
		mutate := func(r workspace.CommandRequest) {
			t.Helper()
			serial++
			r.ActorID = f.actor
			r.RequestID = fmt.Sprintf("%s-parent-%d", id, serial)
			parent, e = f.service.ApplyCommand(t.Context(), parent.Document.ID, r)
			if e != nil {
				t.Fatal(e)
			}
		}
		for i := 0; i < 2; i++ {
			mutate(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: id})
		}
		mutate(workspace.CommandRequest{Type: "SET_REFERENCE_MODE", InstanceID: parent.Product.Instances[1].ID, ReferenceMode: "PINNED"})
		if len(parent.ConstraintDisplayScopes) != 2 {
			t.Fatal("missing nested display projections", parent.ConstraintDisplayScopes)
		}
		before := parent.ConstraintDisplayScopes
		if before[0].TreeNodeID == before[1].TreeNodeID || !strings.HasPrefix(before[0].TreeNodeID, "document:"+parent.Document.ID+"/instance:") {
			t.Fatal("display projection lost semantic tree navigation", before)
		}
		if before[0].InstancePath.Canonical == before[1].InstancePath.Canonical || before[0].Constraints[0].ID != cid || before[1].Constraints[0].ID != cid {
			t.Fatal("aliased owner scopes", before)
		}
		apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{cid}, Suppressed: boolPointer(true)})
		read, e := f.service.GetDocument(t.Context(), parent.Document.ID)
		if e != nil {
			t.Fatal(e)
		}
		for _, scope := range read.ConstraintDisplayScopes {
			if scope.Constraints[0].Suppressed {
				t.Fatal("unaccepted child Head leaked into render", scope)
			}
		}
		mutate(workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
		active, pinned := 0, 0
		for _, scope := range parent.ConstraintDisplayScopes {
			if scope.VersionID == product.Document.VersionID && scope.Constraints[0].Suppressed {
				active++
			} else if scope.VersionID == before[1].VersionID && !scope.Constraints[0].Suppressed {
				pinned++
			}
		}
		if active != 1 || pinned != 1 {
			t.Fatal("FOLLOW accepted projection/PINNED isolation", parent.ConstraintDisplayScopes)
		}
	})
}
