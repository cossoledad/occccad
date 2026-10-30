//go:build integration

package control

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type insertionScene struct {
	t             *testing.T
	s             *workspace.Service
	artifacts     *artifact.Service
	db            database.DB
	client        *geometry.Client
	v             workspace.DocumentView
	base, chamfer workspace.Feature
	seq           int
}

type contextInsertionScene struct {
	*insertionScene
	root, source workspace.DocumentView
	height       string
	active       string
}

func newContextInsertionScene(t *testing.T, repeatedHeight float64) *contextInsertionScene {
	x := newInsertionScene(t)
	height := ""
	for _, p := range x.v.Part.Parameters {
		if p.OwnerFeatureID == x.base.ID && p.PropertySlot == "length" {
			height = p.ParameterID
		}
	}
	x.apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: height, Value: 10, Unit: "mm"})
	x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "YZ"})
	create := func(typ, name string) workspace.DocumentView {
		v, e := x.s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: typ, Name: name})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	c := &contextInsertionScene{insertionScene: x, source: create("PART", "Historical layout"), root: create("PRODUCT", "Context sketch insertion"), height: height}
	c.run(&c.source, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "height", Value: 20, Unit: "mm"})
	sourceHeight := c.source.Part.Parameters[0].ParameterID
	c.run(&c.source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Height", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: sourceHeight})
	publications := []string{c.source.Part.Publications[0].ID}
	c.run(&c.source, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "otherHeight", Value: repeatedHeight, Unit: "mm"})
	otherHeight := ""
	for _, p := range c.source.Part.Parameters {
		if p.ParameterID != sourceHeight {
			otherHeight = p.ParameterID
		}
	}
	c.run(&c.source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Other height", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: otherHeight})
	for _, p := range c.source.Part.Publications {
		if p.ID != publications[0] {
			publications = append(publications, p.ID)
		}
	}
	c.run(&c.root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: c.source.Document.ID})
	for i := 0; i < 2; i++ {
		c.run(&c.root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: x.v.Document.ID})
	}
	c.active = c.root.Product.Instances[1].ID
	c.run(&c.root, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: c.active, Translation: [3]float64{30, 40, 50}, Rotation: [4]float64{0, 0, math.Sqrt(.5), math.Sqrt(.5)}})
	for i := 1; i <= 2; i++ {
		a, e := x.s.GetProductDesignSession(t.Context(), c.root.Document.ID, c.root.Product.Instances[0].ID)
		if e != nil {
			t.Fatal(e)
		}
		b, e := x.s.GetProductDesignSession(t.Context(), c.root.Document.ID, c.root.Product.Instances[i].ID)
		if e != nil {
			t.Fatal(e)
		}
		binding := workspace.ProductContextBindingRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-bind-%d", c.root.Document.ID, i), OwningInstancePath: *b.ActiveInstancePath, SourceInstancePath: *a.ActiveInstancePath, PublicationID: publications[i-1], PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: height}
		if i == 2 {
			binding.ContextInputID = c.v.Part.ContextInputs[0].ID
			binding.TargetKind = ""
			binding.TargetID = ""
		}
		c.root, e = x.s.CreateProductContextBinding(t.Context(), c.root.Document.ID, binding)
		if e != nil {
			t.Fatal(e)
		}
		c.reload()
	}
	// Source Head can change; all editing keeps the explicit accepted values.
	c.run(&c.source, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: sourceHeight, Value: 25, Unit: "mm"})
	c.reload()
	return c
}
func (c *contextInsertionScene) run(v *workspace.DocumentView, r workspace.CommandRequest) {
	c.t.Helper()
	n, e := c.s.ApplyCommand(c.t.Context(), v.Document.ID, c.request(r))
	if e != nil {
		c.t.Fatal(r.Type, e)
	}
	*v = n
}
func (c *contextInsertionScene) reload() {
	c.t.Helper()
	var e error
	c.root, e = c.s.GetDocument(c.t.Context(), c.root.Document.ID, p6Actor)
	if e != nil {
		c.t.Fatal(e)
	}
	c.v, e = c.s.GetDocument(c.t.Context(), c.v.Document.ID, p6Actor)
	if e != nil {
		c.t.Fatal(e)
	}
}
func (c *contextInsertionScene) variant() workspace.ContextVariantSnapshot {
	c.t.Helper()
	for _, v := range c.root.ContextVariants {
		if v.OwningInstancePath.Segments[0].InstanceID == c.active {
			if v.Status != "READY" {
				c.t.Fatal("context not READY", v.Status)
			}
			return v
		}
	}
	c.t.Fatal("active variant missing")
	return workspace.ContextVariantSnapshot{}
}
func (c *contextInsertionScene) scope() workspace.ContextReadScope {
	v := c.variant()
	return workspace.ContextReadScope{RootProductDocumentID: c.root.Document.ID, RootProductRevisionID: c.root.Document.VersionID, ContextVariantID: v.VariantKey, InstancePath: &v.OwningInstancePath}
}
func (c *contextInsertionScene) scoped(r workspace.CommandRequest) workspace.CommandRequest {
	scope := c.scope()
	r.RootProductDocumentID = scope.RootProductDocumentID
	r.RootProductRevisionID = scope.RootProductRevisionID
	r.ContextVariantID = scope.ContextVariantID
	r.InstancePath = scope.InstancePath
	r.VersionID = c.v.Document.VersionID
	return c.request(r)
}
func (c *contextInsertionScene) input(after string) workspace.FeatureInput {
	c.t.Helper()
	v, e := c.s.GetFeatureInput(c.t.Context(), c.v.Document.ID, workspace.FeatureInputRequest{ContextReadScope: c.scope(), VersionID: c.v.Document.VersionID, AfterFeatureID: after, BodyID: c.base.BodyID})
	if e != nil {
		c.t.Fatal(e)
	}
	return v
}

func TestInsertContextSketchAndFiniteExtrudeThroughRouter(t *testing.T) {
	for _, operation := range []string{"ADD", "REMOVE"} {
		for _, face := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/face=%v", operation, face), func(t *testing.T) {
				c := newContextInsertionScene(t, 24)
				ctx := t.Context()
				before := c.v
				rootBefore := c.root
				input := c.input(c.base.ID)
				if input.Artifact.GeometryKey == c.insertionScene.input(c.base.ID, c.base.BodyID).Artifact.GeometryKey {
					t.Fatal("10 -> 20 fixture did not change prefix")
				}
				r := c.scoped(workspace.CommandRequest{Type: "CREATE_SKETCH", AfterFeatureID: c.base.ID, BodyID: c.base.BodyID, GeometryKey: input.Artifact.GeometryKey, Plane: "XY"})
				if face {
					for local := uint64(1); local <= p6TopologyCount(t, input.Artifact.Topology, "faces"); local++ {
						p, e := c.s.GetFeatureInput(ctx, c.v.Document.ID, workspace.FeatureInputRequest{ContextReadScope: c.scope(), VersionID: c.v.Document.VersionID, AfterFeatureID: c.base.ID, BodyID: c.base.BodyID, GeometryKey: input.Artifact.GeometryKey, Kind: "FACE", LocalID: local})
						if e != nil {
							t.Fatal(e)
						}
						if math.Abs(p.Selection.Selection.CreationEvidence.Centroid[2]-10) < 1e-7 {
							r.TargetKind = "FACE"
							r.TopologyID = local
							break
						}
					}
					if r.TopologyID == 0 {
						t.Fatal("top projected face missing")
					}
				}
				for _, mutate := range []func(*workspace.CommandRequest){func(q *workspace.CommandRequest) {
					q.GeometryKey = c.insertionScene.input(c.base.ID, c.base.BodyID).Artifact.GeometryKey
				}, func(q *workspace.CommandRequest) { q.RootProductRevisionID = c.source.Document.VersionID }, func(q *workspace.CommandRequest) { q.ContextVariantID = "forged" }, func(q *workspace.CommandRequest) { q.RootProductDocumentID = "" }, func(q *workspace.CommandRequest) {
					p := *q.InstancePath
					p.Segments = append([]workspace.InstancePathSegment(nil), p.Segments...)
					p.Segments[0].ResolvedVersionID = c.source.Document.VersionID
					q.InstancePath = &p
				}} {
					bad := r
					mutate(&bad)
					c.reject(bad, "")
				}
				preview, e := c.s.PreviewCommand(ctx, c.v.Document.ID, r)
				if e != nil || preview.ReferenceGeometry == nil {
					t.Fatal("context sketch preview", e)
				}
				plane := preview.ReferenceGeometry.DatumPlanes[0]
				if face && math.Abs(plane.Origin[2]-20) > 1e-7 {
					t.Fatal("preview used independent support frame", plane.Origin)
				}
				c.reload()
				if c.v.Document.VersionID != before.Document.VersionID || c.root.Document.VersionID != rootBefore.Document.VersionID {
					t.Fatal("preview wrote document heads")
				}
				if face {
					r.PreviewID = preview.PreviewID
				}
				c.apply(r)
				c.reload()
				sketch := c.v.Part.Features[2]
				if sketch.Sketch == nil || sketch.ID == before.Part.Features[len(before.Part.Features)-1].ID {
					t.Fatal("created wrong sketch")
				}
				if face && (sketch.Sketch.Support.SourceFeatureID != c.base.ID || math.Abs(sketch.Sketch.Support.Origin[2]-10) > 1e-7) {
					t.Fatal("projected support saved as default", sketch.Sketch.Support)
				}
				oldScope := c.scope()
				for _, ops := range [][]workspace.SketchOperation{{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -2, Y: -2}, Second: &workspace.SketchPoint2{X: 2, Y: 2}}}, {{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: sketch.ID + "-construction", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 7, Y: 7}}}}} {
					c.apply(c.scoped(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: ops}))
					c.reload()
				}
				if _, e = c.s.GetFeatureInput(ctx, c.v.Document.ID, workspace.FeatureInputRequest{ContextReadScope: oldScope, VersionID: c.v.Document.VersionID, AfterFeatureID: sketch.ID, BodyID: sketch.BodyID}); e == nil {
					t.Fatal("old context accepted after sketch edit")
				}
				input = c.input(sketch.ID)
				if input.SourceFeatureID != c.base.ID {
					t.Fatal("sketch mistaken for solid source")
				}
				length := 5.
				if !face && operation == "ADD" {
					length = 25
				}
				r = c.scoped(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", AfterFeatureID: sketch.ID, SketchID: sketch.ID, BodyID: sketch.BodyID, GeometryKey: input.Artifact.GeometryKey, Generator: "LINEAR_EXTRUDE", Operation: operation, Extent: "FINITE", Length: length, LengthExpression: fmt.Sprintf("%g mm", length), Reversed: face && operation == "REMOVE", ProfileSelection: &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: c.variant().SketchAnalyses[sketch.ID].RegionIDs}})
				beforeGenerator := c.v
				beforeRoot := c.root
				preview, e = c.s.PreviewCommand(ctx, c.v.Document.ID, r)
				if e != nil || preview.Artifact == nil {
					t.Fatal("context generator preview", e)
				}
				cold, e := c.s.PreviewCommand(workspace.WithColdEvaluation(ctx), c.v.Document.ID, r)
				if e != nil || cold.Artifact == nil {
					t.Fatal("cold context generator", e)
				}
				for _, role := range []string{"BREP", "NAMING"} {
					if preview.Artifact.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
						t.Fatal("context hot/cold mismatch", role)
					}
				}
				c.s.DiscardPreview(c.v.Document.ID, p6Actor, preview.PreviewID)
				c.reload()
				if c.v.Document.VersionID != beforeGenerator.Document.VersionID || c.root.Document.VersionID != beforeRoot.Document.VersionID {
					t.Fatal("cancel wrote head")
				}
				preview, e = c.s.PreviewCommand(ctx, c.v.Document.ID, r)
				if e != nil {
					t.Fatal(e)
				}
				if face {
					r.PreviewID = preview.PreviewID
				}
				c.apply(r)
				c.reload()
				created := c.v.Part.Features[3]
				if created.Profile != sketch.ID || created.BodyID != sketch.BodyID {
					t.Fatal("generator identity/profile mismatch")
				}
				actual := c.variant()
				body := actual.Part.Bodies[0]
				for _, role := range []string{"BREP", "NAMING"} {
					if c.root.Artifacts[body.GeometryKey].Representations[role].Digest != preview.Artifact.Representations[role].Digest {
						t.Fatal("adopted context geometry differs from preview", role)
					}
				}
				prior := beforeRoot.ContextVariants[0]
				for _, v := range beforeRoot.ContextVariants {
					if v.OwningInstancePath.Canonical == actual.OwningInstancePath.Canonical {
						prior = v
					}
				}
				priorVolume := beforeRoot.Artifacts[prior.Part.Bodies[0].GeometryKey].Volume
				finalVolume := c.root.Artifacts[body.GeometryKey].Volume
				if math.Abs(math.Abs(finalVolume-priorVolume)-80) > 1e-5 || operation == "ADD" && finalVolume <= priorVolume || operation == "REMOVE" && finalVolume >= priorVolume {
					t.Fatal("wrong context material delta", finalVolume, priorVolume)
				}
				if face {
					for _, f := range actual.Part.Features {
						if f.ID == sketch.ID && math.Abs(f.Sketch.Support.Origin[2]-20) > 1e-7 {
							t.Fatal("context sketch plane did not follow accepted support")
						}
					}
				}
				for _, variant := range c.root.ContextVariants {
					height := 0.
					for _, f := range variant.Part.Features {
						if f.ID == c.base.ID {
							height = f.Length
						}
					}
					if height != 20 && height != 24 {
						t.Fatal("accepted input read source Head", height)
					}
				}
				active := c.root.Product.Instances[1]
				if active.Translation != rootBefore.Product.Instances[1].Translation || active.Rotation != rootBefore.Product.Instances[1].Rotation {
					t.Fatal("creation changed occurrence pose")
				}
				for i, b := range beforeRoot.Product.ContextBindings {
					if b.Accepted.SourceRevisionID != c.root.Product.ContextBindings[i].Accepted.SourceRevisionID {
						t.Fatal("unaccepted update advanced")
					}
				}
				c.apply(workspace.CommandRequest{Type: "UNDO"})
				c.reload()
				if !reflect.DeepEqual(c.v.Part.Features, beforeGenerator.Part.Features) {
					t.Fatal("Undo did not restore sketch/order")
				}
				c.apply(workspace.CommandRequest{Type: "REDO"})
				c.reload()
				if c.v.Part.Features[3].ID != created.ID {
					t.Fatal("Redo changed identity")
				}
				fresh := workspace.NewWithArtifacts(c.db, c.client, c.artifacts)
				opened, e := fresh.GetDocument(ctx, c.root.Document.ID, p6Actor)
				if e != nil || len(opened.ContextVariants) != 2 {
					t.Fatal("reopen", e)
				}
				for _, scoped := range []bool{false, true} {
					q := workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: created.ID, Feature: &created, ExpectedFeatureDigest: findP6FeatureDigest(t, c.v.StructureTree, created.ID)}
					if scoped {
						q = c.scoped(q)
					} else {
						q = c.request(q)
					}
					p, e := fresh.PreviewCommand(workspace.WithColdEvaluation(ctx), c.v.Document.ID, q)
					if e != nil || p.Artifact == nil {
						t.Fatal("reopened cold reconstruction", scoped, e)
					}
					want := activeBodyArtifact(t, c.v)
					if scoped {
						want = c.root.Artifacts[c.variant().Part.Bodies[0].GeometryKey]
					}
					for _, role := range []string{"BREP", "NAMING"} {
						if want.Representations[role].Digest != p.Artifact.Representations[role].Digest {
							t.Fatal("reopened configuration mismatch", scoped, role)
						}
					}
				}
			})
		}
	}
}

func TestInsertContextSketchRepeatedOccurrenceFailureIsAtomicThroughRouter(t *testing.T) {
	c := newContextInsertionScene(t, 5)
	input := c.input(c.base.ID)
	c.apply(c.scoped(workspace.CommandRequest{Type: "CREATE_SKETCH", AfterFeatureID: c.base.ID, BodyID: c.base.BodyID, GeometryKey: input.Artifact.GeometryKey, Plane: "XY"}))
	c.reload()
	sketch := c.v.Part.Features[2]
	c.apply(c.scoped(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -11, Y: -11}, Second: &workspace.SketchPoint2{X: 11, Y: 11}}}}))
	c.reload()
	input = c.input(sketch.ID)
	r := c.scoped(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", AfterFeatureID: sketch.ID, BodyID: sketch.BodyID, SketchID: sketch.ID, GeometryKey: input.Artifact.GeometryKey, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Extent: "FINITE", Length: 6})
	beforeRoot := c.root.Document.VersionID
	c.reject(r, "")
	c.reload()
	if c.root.Document.VersionID != beforeRoot {
		t.Fatal("failed repeated occurrence advanced Product")
	}
}

func TestInsertContextDefaultPublicationFailureRejectsPreviewThroughRouter(t *testing.T) {
	c := newContextInsertionScene(t, 24)
	c.apply(c.scoped(workspace.CommandRequest{Type: "DELETE_NODE", TargetKind: "FEATURE", TargetID: c.chamfer.ID}))
	c.reload()
	pick := findP7TopologyPick(t, c.s, c.v, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return p.CreationEvidence.GeometryType == "PLANE" && math.Abs(p.CreationEvidence.Centroid[2]-10) < 1e-7
	})
	c.apply(c.scoped(workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Default top face", PublicationType: "SURFACE", TargetKind: "FACE", GeometryKey: pick.GeometryKey, TopologyID: pick.LocalID}))
	c.reload()
	input := c.input(c.base.ID)
	c.apply(c.scoped(workspace.CommandRequest{Type: "CREATE_SKETCH", AfterFeatureID: c.base.ID, BodyID: c.base.BodyID, GeometryKey: input.Artifact.GeometryKey, Plane: "XY"}))
	c.reload()
	sketch := c.v.Part.Features[2]
	c.apply(c.scoped(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -12, Y: -12}, Second: &workspace.SketchPoint2{X: 12, Y: 12}}}}))
	c.reload()
	input = c.input(sketch.ID)
	// The default 10 mm top disappears inside the larger 15 mm pad. Accepted
	// 20/24 mm tops survive and their geometry alone would incorrectly preview.
	r := c.scoped(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", AfterFeatureID: sketch.ID, SketchID: sketch.ID, BodyID: sketch.BodyID, GeometryKey: input.Artifact.GeometryKey, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Extent: "FINITE", Length: 15})
	head := c.root.Document.VersionID
	c.reject(r, "Default top face")
	c.reload()
	if c.root.Document.VersionID != head {
		t.Fatal("publication rejection changed Product")
	}
}

func (x *insertionScene) historicalSketch(after string, face bool, adopt bool) workspace.Feature {
	x.t.Helper()
	input := x.input(after, x.base.BodyID)
	r := x.request(workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: x.base.BodyID, AfterFeatureID: after, VersionID: x.v.Document.VersionID, GeometryKey: input.Artifact.GeometryKey, Plane: "XY"})
	if face {
		for local := uint64(1); local <= p6TopologyCount(x.t, input.Artifact.Topology, "faces"); local++ {
			pick, e := x.s.GetFeatureInput(x.t.Context(), x.v.Document.ID, workspace.FeatureInputRequest{VersionID: r.VersionID, AfterFeatureID: after, BodyID: x.base.BodyID, GeometryKey: r.GeometryKey, Kind: "FACE", LocalID: local})
			if e != nil {
				x.t.Fatal(e)
			}
			if math.Abs(pick.Selection.Selection.CreationEvidence.Centroid[2]-20) < 1e-8 {
				r.TargetKind = "FACE"
				r.TopologyID = local
				break
			}
		}
		if r.TopologyID == 0 {
			x.t.Fatal("top historical support missing")
		}
	}
	before := x.v
	preview, e := x.s.PreviewCommand(x.t.Context(), x.v.Document.ID, r)
	if e != nil || preview.PreviewID == "" || preview.ReferenceGeometry == nil {
		x.t.Fatal("sketch creation preview", e)
	}
	check, e := x.s.GetDocument(x.t.Context(), x.v.Document.ID, p6Actor)
	if e != nil || check.Document.VersionID != before.Document.VersionID {
		x.t.Fatal("sketch preview advanced Head", e)
	}
	if adopt {
		r.PreviewID = preview.PreviewID
	}
	x.apply(r)
	known := map[string]bool{}
	for _, f := range before.Part.Features {
		known[f.ID] = true
	}
	var created workspace.Feature
	for _, f := range x.v.Part.Features {
		if !known[f.ID] && f.Sketch != nil {
			if created.ID != "" {
				x.t.Fatal("multiple sketches created")
			}
			created = f
		}
	}
	if created.ID == "" || created.BodyID != x.base.BodyID {
		x.t.Fatal("new sketch identity/Body missing")
	}
	if face && (created.Sketch.Support.SourceFeatureID != after || created.Sketch.Support.SourceVersionID != r.VersionID) {
		x.t.Fatal("support lost its actual stage")
	}
	return created
}

func (x *insertionScene) historicalExtrude(sketch workspace.Feature, operation string, length float64, reverse bool) workspace.CommandRequest {
	x.t.Helper()
	input := x.input(sketch.ID, sketch.BodyID)
	if input.SourceFeatureID == sketch.ID || input.SourceFeatureID != x.base.ID {
		x.t.Fatal("global sketch boundary was mistaken for a solid source", input.SourceFeatureID)
	}
	return x.request(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", AfterFeatureID: sketch.ID, SketchID: sketch.ID, BodyID: sketch.BodyID, VersionID: x.v.Document.VersionID, GeometryKey: input.Artifact.GeometryKey, Generator: "LINEAR_EXTRUDE", Operation: operation, Extent: "FINITE", Length: length, Reversed: reverse,
		ProfileSelection: &workspace.ProfileRegionSelection{Mode: "REGIONS", RegionIDs: append([]string(nil), x.v.SketchAnalyses[sketch.ID].RegionIDs...)}})
}

func TestInsertSketchAndFiniteExtrudeLifecycleThroughRouter(t *testing.T) {
	for _, operation := range []string{"ADD", "REMOVE"} {
		for _, face := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/face=%v", operation, face), func(t *testing.T) {
				x := newInsertionScene(t)
				// A later sketch is intentionally last, before and after insertion.
				x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "YZ"})
				later := x.v.Part.Features[len(x.v.Part.Features)-1].ID
				baseline := x.v
				sketch := x.historicalSketch(x.base.ID, face, face)
				if x.v.Part.Features[len(x.v.Part.Features)-1].ID != later || x.v.Part.Features[2].ID != sketch.ID {
					t.Fatal("sketch was appended instead of inserted")
				}
				oldRevision := x.v.Document.VersionID
				x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -2, Y: -2}, Second: &workspace.SketchPoint2{X: 2, Y: 2}}}})
				x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: sketch.ID + "-construction", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 7, Y: 7}}}}})
				if _, e := x.s.GetFeatureInput(t.Context(), x.v.Document.ID, workspace.FeatureInputRequest{VersionID: oldRevision, AfterFeatureID: sketch.ID, BodyID: sketch.BodyID}); e == nil {
					t.Fatal("pre-edit input was accepted")
				}
				length := 25.0
				if operation == "REMOVE" {
					length = 5
				}
				if face {
					length = 5
				}
				r := x.historicalExtrude(sketch, operation, length, face && operation == "REMOVE")
				r.LengthExpression = fmt.Sprintf("%g mm", length)
				beforeGenerator := x.v
				preview, e := x.s.PreviewCommand(t.Context(), x.v.Document.ID, r)
				if e != nil || preview.Artifact == nil {
					t.Fatal("extrude preview", e)
				}
				x.s.DiscardPreview(x.v.Document.ID, p6Actor, preview.PreviewID)
				canceled, e := x.s.GetDocument(t.Context(), x.v.Document.ID, p6Actor)
				if e != nil || canceled.Document.VersionID != beforeGenerator.Document.VersionID || !slices.ContainsFunc(canceled.Part.Features, func(f workspace.Feature) bool { return f.ID == sketch.ID && len(f.Sketch.Entities) > 0 }) {
					t.Fatal("generator cancellation did not retain committed sketch", e)
				}
				if _, e = x.s.ApplyCommand(t.Context(), x.v.Document.ID, func() workspace.CommandRequest { stale := r; stale.PreviewID = preview.PreviewID; return stale }()); e == nil {
					t.Fatal("discarded preview committed")
				}
				preview, e = x.s.PreviewCommand(t.Context(), x.v.Document.ID, r)
				if e != nil || preview.Artifact == nil {
					t.Fatal(e)
				}
				cold, e := x.s.PreviewCommand(workspace.WithColdEvaluation(t.Context()), x.v.Document.ID, r)
				if e != nil || cold.Artifact == nil {
					t.Fatal("cold candidate", e)
				}
				for _, role := range []string{"BREP", "NAMING"} {
					if preview.Artifact.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
						t.Fatal("candidate hot/cold mismatch", role)
					}
				}
				if face {
					r.PreviewID = preview.PreviewID
				}
				x.apply(r)
				created := x.v.Part.Features[3]
				if created.Type != "LINEAR_EXTRUDE" || created.Profile != sketch.ID || created.BodyID != sketch.BodyID || created.ID == sketch.ID {
					t.Fatal("wrong created generator")
				}
				for i, f := range x.v.Part.Features {
					if f.Order != i+1 || f.EvaluationStatus != "" {
						t.Fatal("order or downstream evaluation failed", f.Name, f.Diagnostic)
					}
				}
				final := activeBodyArtifact(t, x.v)
				original := activeBodyArtifact(t, baseline)
				if operation == "ADD" && !(final.Volume > original.Volume) || operation == "REMOVE" && !(final.Volume < original.Volume) {
					t.Fatal("wrong material result")
				}
				if math.Abs(math.Abs(final.Volume-original.Volume)-80) > 1e-5 {
					t.Fatal("wrong exact material volume", final.Volume, original.Volume)
				}
				if final.Representations["BREP"].Digest != preview.Artifact.Representations["BREP"].Digest || final.Representations["NAMING"].Digest != preview.Artifact.Representations["NAMING"].Digest {
					t.Fatal("commit differs from preview")
				}
				x.apply(workspace.CommandRequest{Type: "UNDO"})
				if !reflect.DeepEqual(x.v.Part.Features, beforeGenerator.Part.Features) || !reflect.DeepEqual(x.v.Part.Parameters, beforeGenerator.Part.Parameters) {
					t.Fatal("one Undo lost sketch or complete order/parameters")
				}
				x.apply(workspace.CommandRequest{Type: "REDO"})
				if x.v.Part.Features[3].ID != created.ID || activeBodyArtifact(t, x.v).Representations["NAMING"].Digest != final.Representations["NAMING"].Digest {
					t.Fatal("Redo lost stable result")
				}
				fresh := workspace.NewWithArtifacts(x.db, x.client, x.artifacts)
				opened, e := fresh.GetDocument(t.Context(), x.v.Document.ID, p6Actor)
				if e != nil || !reflect.DeepEqual(opened.Part.Features, x.v.Part.Features) {
					t.Fatal("reopen", e)
				}
				rebuilt, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), x.v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: r.RequestID + "-rebuild", Type: "EDIT_FEATURE", TargetID: created.ID, Feature: &created, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, created.ID)})
				if e != nil || rebuilt.Artifact == nil {
					t.Fatal("reopened cold rebuild", e)
				}
				for _, role := range []string{"BREP", "NAMING"} {
					if final.Representations[role].Digest != rebuilt.Artifact.Representations[role].Digest {
						t.Fatal("reopened cold mismatch", role)
					}
				}
				// Sketch creation, each edit and generator creation remain separate transactions.
				for i := 0; i < 4; i++ {
					x.apply(workspace.CommandRequest{Type: "UNDO"})
				}
				if !reflect.DeepEqual(x.v.Part.Features, baseline.Part.Features) || !reflect.DeepEqual(x.v.Part.Parameters, baseline.Part.Parameters) {
					t.Fatal("sketch undo did not restore original global order")
				}
				for i := 0; i < 4; i++ {
					x.apply(workspace.CommandRequest{Type: "REDO"})
				}
				if x.v.Part.Features[2].ID != sketch.ID || x.v.Part.Features[3].ID != created.ID {
					t.Fatal("redo changed sketch/generator identities")
				}
			})
		}
	}
}

func TestInsertSketchGeneratorSourceAndRangeGuardsThroughRouter(t *testing.T) {
	t.Run("frozen-input", func(t *testing.T) {
		x := newInsertionScene(t)
		input := x.input(x.base.ID, x.base.BodyID)
		r := workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: x.base.BodyID, AfterFeatureID: x.base.ID, VersionID: x.v.Document.VersionID, GeometryKey: input.Artifact.GeometryKey, Plane: "XY"}
		bad := r
		bad.GeometryKey = activeBodyArtifact(t, x.v).GeometryKey
		x.reject(bad, "GeometryKey")
		bad = r
		bad.TargetKind = "FACE"
		bad.TopologyID = 1
		bad.GeometryKey = activeBodyArtifact(t, x.v).GeometryKey
		x.reject(bad, "OUTSIDE_INPUT")
		x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "YZ"})
		x.reject(r, "Revision")
		sketch := x.historicalSketch(x.base.ID, false, false)
		x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -2, Y: -2}, Second: &workspace.SketchPoint2{X: 2, Y: 2}}}})
		create := x.historicalExtrude(sketch, "ADD", 25, false)
		for _, extent := range []string{"SYMMETRIC", "TWO_SIDED", "THROUGH_ALL", "TO_PLANE"} {
			bad := create
			bad.Extent = extent
			x.reject(bad, "finite LINEAR_EXTRUDE")
		}
		bad = create
		bad.Operation = "NEW_BODY"
		x.reject(bad, "finite LINEAR_EXTRUDE")
		bad = create
		bad.Generator = "REVOLVE"
		bad.Angle = 90
		x.reject(bad, "finite LINEAR_EXTRUDE")
		bad = create
		bad.AfterFeatureID = x.base.ID
		x.reject(bad, "immediately after")
		bad = create
		bad.SketchID = x.v.Part.Features[0].ID
		x.reject(bad, "immediately after")
		bad = create
		bad.GeometryKey = activeBodyArtifact(t, x.v).GeometryKey
		x.reject(bad, "GeometryKey")
		x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: sketch.ID + "-new-point", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 5, Y: 5}}}}})
		x.reject(create, "Revision")
		x.apply(workspace.CommandRequest{Type: "CREATE_BODY"})
		bad = x.historicalExtrude(sketch, "ADD", 25, false)
		bad.BodyID = x.v.Part.ActiveBodyID
		x.reject(bad, "specified Body")
	})
	t.Run("recursive-datum-order", func(t *testing.T) {
		x := newInsertionScene(t)
		pick, _ := x.faceWhere(x.chamfer.ID, x.base.BodyID, x.base.BodyID, false, func(p workspace.FeatureSelection) bool { return p.Selection.CreationEvidence.GeometryType == "PLANE" })
		x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: workspace.DatumReference{Kind: "TOPOLOGY", FeatureID: x.chamfer.ID, Selection: &pick.Selection, SourceVersionID: pick.SourceVersionID}}})
		first := x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1]
		x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: workspace.DatumReference{Kind: "PLANE", EntityID: first.ID}}})
		chained := x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1]
		input := x.input(x.base.ID, x.base.BodyID)
		if input.ReferenceGeometry == nil || slices.ContainsFunc(input.ReferenceGeometry.DatumPlanes, func(p workspace.DatumPlane) bool { return p.ID == first.ID || p.ID == chained.ID }) {
			t.Fatal("downstream datum chain exposed")
		}
		x.reject(workspace.CommandRequest{Type: "CREATE_SKETCH", AfterFeatureID: x.base.ID, BodyID: x.base.BodyID, VersionID: x.v.Document.VersionID, GeometryKey: input.Artifact.GeometryKey, DatumPlaneID: chained.ID}, "DATUM_DOWNSTREAM_REFERENCE")
	})
	t.Run("protected-range", func(t *testing.T) {
		x := newInsertionScene(t)
		x.apply(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &workspace.Feature{Type: "SOLID_PATTERN", BodyID: x.base.BodyID, Operation: "ADD", Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Direction: [3]float64{1, 0, 0}, Count: 2, Spacing: 30}, SourceKind: "FEATURE_DELTA", StartFeatureID: x.base.ID, Source: workspace.FeatureStageRef{BodyID: x.base.BodyID, FeatureID: x.chamfer.ID}, ResultMode: "COMBINE"}}})
		sketch := x.historicalSketch(x.base.ID, false, true)
		x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -2, Y: -2}, Second: &workspace.SketchPoint2{X: 2, Y: 2}}}})
		x.reject(x.historicalExtrude(sketch, "ADD", 25, false), "FEATURE_DELTA")
	})
	t.Run("downstream-failure", func(t *testing.T) {
		x := newInsertionScene(t)
		sketch := x.historicalSketch(x.base.ID, false, false)
		x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 8, Y: 8}, Second: &workspace.SketchPoint2{X: 12, Y: 12}}}})
		x.reject(x.historicalExtrude(sketch, "REMOVE", 20, false), x.chamfer.Name)
	})
	t.Run("cross-body-consumer", func(t *testing.T) {
		x := newInsertionScene(t)
		x.apply(workspace.CommandRequest{Type: "CREATE_BODY"})
		bodyB := x.v.Part.ActiveBodyID
		x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		source := x.v.Part.Features[len(x.v.Part.Features)-1].ID
		x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: source, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10.2, Y: -10.2}, Second: &workspace.SketchPoint2{X: -9.8, Y: -9.8}}}})
		x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: source, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
		x.apply(workspace.CommandRequest{Type: "CREATE_BOOLEAN_FEATURE", BodyID: bodyB, Operation: "INTERSECT", KeepTools: true, Tools: []workspace.FeatureStageRef{{BodyID: x.base.BodyID, FeatureID: x.chamfer.ID}}})
		consumer := x.v.Part.Features[len(x.v.Part.Features)-1]
		sketch := x.historicalSketch(x.base.ID, false, false)
		x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -11, Y: -11}, Second: &workspace.SketchPoint2{X: -9, Y: -9}}}})
		x.reject(x.historicalExtrude(sketch, "REMOVE", 20, false), consumer.Name)
	})
}

func TestInsertedSketchProjectionUsesActualUpstreamStageThroughRouter(t *testing.T) {
	x := newInsertionScene(t)
	sketch := x.historicalSketch(x.base.ID, false, false)
	input, e := x.s.GetFeatureInput(t.Context(), x.v.Document.ID, workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, FeatureID: sketch.ID, BodyID: sketch.BodyID})
	if e != nil || input.SourceFeatureID != x.base.ID {
		t.Fatal("sketch input", e)
	}
	local := uint64(1)
	projection := workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_EXTERNAL_GEOMETRY", ExternalID: sketch.ID + "-projection", GeometryKey: input.Artifact.GeometryKey, TopologyKind: "EDGE", TopologyID: local, SourceVersionID: x.v.Document.VersionID}}}
	wrong := projection
	wrong.Operations = append([]workspace.SketchOperation(nil), projection.Operations...)
	wrong.Operations[0].GeometryKey = activeBodyArtifact(t, x.v).GeometryKey
	x.reject(wrong, "outside sketch upstream")
	x.apply(projection)
	for _, f := range x.v.Part.Features {
		if f.ID == sketch.ID {
			external := f.Sketch.ExternalGeometry[0]
			if external.SourceFeatureID != x.base.ID || external.Status != "CONNECTED" || external.Snapshot == nil {
				t.Fatal("projection lost exact source stage", external)
			}
		}
	}
	x.reject(projection, "geometryKey")
	x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -2, Y: -2}, Second: &workspace.SketchPoint2{X: 2, Y: 2}}}})
	r := x.historicalExtrude(sketch, "ADD", 25, false)
	x.apply(r)
	final := activeBodyArtifact(t, x.v)
	created := x.v.Part.Features[3]
	preview, e := workspace.NewWithArtifacts(x.db, x.client, x.artifacts).PreviewCommand(workspace.WithColdEvaluation(t.Context()), x.v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: r.RequestID + "-projection-cold", Type: "EDIT_FEATURE", TargetID: created.ID, Feature: &created, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, created.ID)})
	if e != nil || preview.Artifact == nil {
		t.Fatal("cold projection rebuild", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if final.Representations[role].Digest != preview.Artifact.Representations[role].Digest {
			t.Fatal("projection cold mismatch", role)
		}
	}
}

func newInsertionScene(t *testing.T) *insertionScene {
	t.Helper()
	s, a, db, c, _ := featureAssociationRuntimeService(t)
	v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Insert local modifier"})
	if e != nil {
		t.Fatal(e)
	}
	x := &insertionScene{t: t, s: s, artifacts: a, db: db, client: c, v: v}
	x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := x.v.Part.Features[0].ID
	x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
	x.base = x.v.Part.Features[len(x.v.Part.Features)-1]
	p := findP7TopologyPick(t, s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return p.CreationEvidence.Centroid == [3]float64{10, 10, 10}
	})
	x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: x.base.BodyID, Length: 1, Selections: []workspace.FeatureSelection{{Selection: p.Selection, SourceVersionID: p.SourceVersionID}}}})
	x.chamfer = x.v.Part.Features[len(x.v.Part.Features)-1]
	return x
}
func (x *insertionScene) request(r workspace.CommandRequest) workspace.CommandRequest {
	x.seq++
	r.ActorID = p6Actor
	if r.RequestID == "" {
		r.RequestID = fmt.Sprintf("%s-insert-%d", x.v.Document.ID, x.seq)
	}
	return r
}
func (x *insertionScene) apply(r workspace.CommandRequest) {
	x.t.Helper()
	v, e := x.s.ApplyCommand(x.t.Context(), x.v.Document.ID, x.request(r))
	if e != nil {
		x.t.Fatal(r.Type, e)
	}
	x.v = v
}
func (x *insertionScene) input(after, body string) workspace.FeatureInput {
	x.t.Helper()
	r, e := x.s.GetFeatureInput(x.t.Context(), x.v.Document.ID, workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, AfterFeatureID: after, BodyID: body})
	if e != nil {
		x.t.Fatal(e)
	}
	return r
}
func (x *insertionScene) pick(after string, centroid [3]float64) (workspace.FeatureSelection, string) {
	x.t.Helper()
	input := x.input(after, x.base.BodyID)
	for local := uint64(1); local <= p6TopologyCount(x.t, input.Artifact.Topology, "edges"); local++ {
		r, e := x.s.GetFeatureInput(x.t.Context(), x.v.Document.ID, workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, AfterFeatureID: after, BodyID: x.base.BodyID, GeometryKey: input.Artifact.GeometryKey, Kind: "EDGE", LocalID: local})
		if e != nil {
			x.t.Fatal(e)
		}
		if r.Selection.Selection.CreationEvidence.Centroid == centroid {
			return *r.Selection, input.Artifact.GeometryKey
		}
	}
	x.t.Fatal("stage edge missing", centroid)
	return workspace.FeatureSelection{}, ""
}
func (x *insertionScene) insert(after string, point [3]float64, radius float64) workspace.CommandRequest {
	p, key := x.pick(after, point)
	return x.request(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", AfterFeatureID: after, VersionID: x.v.Document.VersionID, GeometryKey: key, Feature: &workspace.Feature{Type: "FILLET", BodyID: x.base.BodyID, Length: radius, Selections: []workspace.FeatureSelection{p}}})
}
func (x *insertionScene) reject(r workspace.CommandRequest, reason string) {
	x.t.Helper()
	r = x.request(r)
	head := x.v.Document.VersionID
	for _, preview := range []bool{true, false} {
		var e error
		if preview {
			_, e = x.s.PreviewCommand(x.t.Context(), x.v.Document.ID, r)
		} else {
			_, e = x.s.ApplyCommand(x.t.Context(), x.v.Document.ID, r)
		}
		if e == nil || !strings.Contains(e.Error(), reason) {
			x.t.Fatal("expected atomic rejection", reason, e)
		}
	}
	v, e := x.s.GetDocument(x.t.Context(), x.v.Document.ID, p6Actor)
	if e != nil || v.Document.VersionID != head {
		x.t.Fatal("rejection advanced Head", e)
	}
}

func TestInsertModifierLifecycleThroughRouter(t *testing.T) {
	x := newInsertionScene(t)
	tctx := t.Context()
	head := x.v.Document.VersionID
	input := x.input(x.base.ID, x.base.BodyID)
	if input.SourceFeatureID != x.base.ID || math.Abs(input.Artifact.Volume-8000) > 1e-6 || input.Artifact.GeometryKey == activeBodyArtifact(t, x.v).GeometryKey {
		t.Fatal("not the exact base stage")
	}
	for _, r := range []workspace.FeatureInputRequest{
		{VersionID: head, AfterFeatureID: x.base.ID, BodyID: x.base.BodyID, GeometryKey: activeBodyArtifact(t, x.v).GeometryKey, Kind: "EDGE", LocalID: 1},
		{VersionID: head, FeatureID: x.base.ID, ResultStage: true, GeometryKey: input.Artifact.GeometryKey, Kind: "EDGE", LocalID: 1},
		{VersionID: head, AfterFeatureID: x.base.ID, BodyID: x.base.BodyID, ResultStage: true},
		{VersionID: head, AfterFeatureID: x.v.Part.Features[0].ID, BodyID: x.base.BodyID},
		{VersionID: head, AfterFeatureID: x.base.ID, BodyID: "wrong-body"},
		{VersionID: head, AfterFeatureID: x.base.ID, BodyID: x.base.BodyID, GeometryKey: input.Artifact.GeometryKey, Kind: "VERTEX", LocalID: 1},
	} {
		if _, e := x.s.GetFeatureInput(tctx, x.v.Document.ID, r); e == nil {
			t.Fatal("illegal pick authorized", r)
		}
	}
	// Consuming a saved downstream chamfer edge is a geometry/reference blocker.
	x.reject(x.insert(x.base.ID, [3]float64{10, 10, 10}, 1), x.chamfer.ID)
	r := x.insert(x.base.ID, [3]float64{-10, -10, 10}, 1)
	bad := r
	f := *r.Feature
	f.Selections = append([]workspace.FeatureSelection(nil), f.Selections...)
	f.Selections[0].SourceFeatureID = x.chamfer.ID
	bad.Feature = &f
	x.reject(bad, "insertion stage")
	bad = r
	bad.GeometryKey = activeBodyArtifact(t, x.v).GeometryKey
	x.reject(bad, "GeometryKey")
	bad = r
	f = *r.Feature
	f.Type = "SHELL"
	bad.Feature = &f
	x.reject(bad, "modifier selection")
	bad = r
	bad.VersionID = "missing-revision"
	x.reject(bad, "insertion stage")
	before := append([]workspace.Feature(nil), x.v.Part.Features...)
	params := append([]modelcore.ParameterDefinition(nil), x.v.Part.Parameters...)
	history, e := x.s.ListHistory(tctx, x.v.Document.ID)
	if e != nil {
		t.Fatal(e)
	}
	r.ParameterExpressions = map[string]string{"length": "0.5 mm + 0.5 mm"}
	preview, e := x.s.PreviewCommand(tctx, x.v.Document.ID, r)
	if e != nil || preview.Artifact == nil {
		t.Fatal("preview", e)
	}
	now, e := x.s.GetDocument(tctx, x.v.Document.ID, p6Actor)
	if e != nil || now.Document.VersionID != head {
		t.Fatal("preview/cancel wrote Head", e)
	}
	stale := r
	f = *r.Feature
	f.Length = 2
	stale.Feature = &f
	stale.ParameterExpressions = nil
	stale.PreviewID = preview.PreviewID
	if _, e = x.s.ApplyCommand(tctx, x.v.Document.ID, stale); e == nil {
		t.Fatal("old preview accepted changed inputs")
	}
	r.PreviewID = preview.PreviewID
	x.apply(r)
	after, e := x.s.ListHistory(tctx, x.v.Document.ID)
	if e != nil || len(after) != len(history)+1 {
		t.Fatal("insertion was not one history step", e)
	}
	created := x.v.Part.Features[2]
	if created.ID == x.chamfer.ID || created.Type != "FILLET" || x.v.Part.Features[3].ID != x.chamfer.ID || created.Selections[0].SourceFeatureID != x.base.ID || created.Selections[0].SourceVersionID != head {
		t.Fatal("middle creation or exact source lost", created)
	}
	for i, f := range x.v.Part.Features {
		if f.Order != i+1 {
			t.Fatal("Order not synchronized")
		}
	}
	for _, p := range params {
		found := false
		for _, q := range x.v.Part.Parameters {
			if p.ParameterID == q.ParameterID {
				found = true
				if !reflect.DeepEqual(p, q) {
					t.Fatal("old parameter modified")
				}
			}
		}
		if !found {
			t.Fatal("old parameter missing")
		}
	}
	for _, old := range before {
		found := false
		for _, f := range x.v.Part.Features {
			if old.ID == f.ID {
				old.Order = f.Order
				found = true
				if !reflect.DeepEqual(old, f) {
					t.Fatal("old feature rebound")
				}
			}
		}
		if !found {
			t.Fatal("old identity lost")
		}
	}
	final := activeBodyArtifact(t, x.v)
	if math.Abs(final.Volume-(7990-20*(1-math.Pi/4))) > 1e-5 {
		t.Fatal("incorrect final geometry", final.Volume)
	}
	stage, e := x.s.GetFeatureInput(tctx, x.v.Document.ID, workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, FeatureID: created.ID, ResultStage: true})
	if e != nil || math.Abs(stage.Artifact.Volume-(8000-20*(1-math.Pi/4))) > 1e-5 {
		t.Fatal("incorrect intermediate stage", e)
	}
	retry, e := x.s.ApplyCommand(tctx, x.v.Document.ID, r)
	if e != nil || retry.Document.VersionID != x.v.Document.VersionID {
		t.Fatal("not idempotent", e)
	}
	x.apply(workspace.CommandRequest{Type: "UNDO"})
	if !reflect.DeepEqual(x.v.Part.Features, before) || !reflect.DeepEqual(x.v.Part.Parameters, params) {
		t.Fatal("one Undo did not restore complete order and parameters")
	}
	x.apply(workspace.CommandRequest{Type: "REDO"})
	if activeBodyArtifact(t, x.v).GeometryID != final.GeometryID || x.v.Part.Features[2].ID != created.ID {
		t.Fatal("Redo lost identity/geometry")
	}
	fresh := workspace.NewWithArtifacts(x.db, x.client, x.artifacts)
	opened, e := fresh.GetDocument(tctx, x.v.Document.ID, p6Actor)
	if e != nil || !reflect.DeepEqual(opened.Part.Features, x.v.Part.Features) {
		t.Fatal("reopen", e)
	}
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(tctx), x.v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: x.v.Document.ID + "-cold", Type: "EDIT_FEATURE", TargetID: created.ID, Feature: &created, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, created.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
			t.Fatal("cold mismatch", role)
		}
	}
	staleHead := r
	staleHead.RequestID += "-stale-head"
	x.reject(staleHead, "")
	// The pre-insertion context expires as soon as Revision changes.
	if _, e = x.s.GetFeatureInput(tctx, x.v.Document.ID, workspace.FeatureInputRequest{VersionID: head, AfterFeatureID: x.base.ID, BodyID: x.base.BodyID}); e == nil {
		t.Fatal("stale insertion pick authorized")
	}
}

func TestInsertModifierRangeAndFixedStageThroughRouter(t *testing.T) {
	x := newInsertionScene(t)
	x.apply(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &workspace.Feature{Type: "SOLID_PATTERN", BodyID: x.base.BodyID, Operation: "ADD", Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Direction: [3]float64{1, 0, 0}, Count: 2, Spacing: 30}, SourceKind: "FEATURE_DELTA", StartFeatureID: x.base.ID, Source: workspace.FeatureStageRef{BodyID: x.base.BodyID, FeatureID: x.chamfer.ID}, ResultMode: "COMBINE"}}})
	pattern := x.v.Part.Features[len(x.v.Part.Features)-1]
	x.reject(x.insert(x.base.ID, [3]float64{-10, -10, 10}, 1), "FEATURE_DELTA")
	for _, kind := range []string{"DRAFT", "SHELL"} {
		x.reject(x.faceInsertion(kind, x.base.ID), "FEATURE_DELTA")
	}
	if p, e := x.s.PreviewCommand(t.Context(), x.v.Document.ID, x.faceInsertion("DRAFT", pattern.ID)); e != nil || p.Artifact == nil {
		t.Fatal("draft after fixed pattern output", e)
	}
	// An unchanged fixed pattern is a legitimate insertion anchor; this is not a move.
	r := x.insert(pattern.ID, [3]float64{-10, -10, 10}, .2)
	r.Feature.Type = "CHAMFER"
	x.apply(r)
	created := x.v.Part.Features[len(x.v.Part.Features)-1]
	if created.Type != "CHAMFER" || created.Selections[0].SourceFeatureID != pattern.ID {
		t.Fatal("fixed output was not a legitimate stage")
	}
}

func TestInsertModifierRejectsCrossBodyFailureThroughRouter(t *testing.T) {
	x := newInsertionScene(t)
	bodyA := x.base.BodyID
	x.apply(workspace.CommandRequest{Type: "CREATE_BODY"})
	bodyB := x.v.Part.ActiveBodyID
	x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := x.v.Part.Features[len(x.v.Part.Features)-1].ID
	x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10.2, Y: -10.2}, Second: &workspace.SketchPoint2{X: -9.8, Y: -9.8}}}})
	x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
	x.apply(workspace.CommandRequest{Type: "CREATE_BOOLEAN_FEATURE", BodyID: bodyB, Operation: "INTERSECT", KeepTools: true, Tools: []workspace.FeatureStageRef{{BodyID: bodyA, FeatureID: x.chamfer.ID}}})
	consumer := x.v.Part.Features[len(x.v.Part.Features)-1]
	x.apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: bodyA})
	// The inserted fillet is valid in A, but removes all material used by B's
	// intersection with the declared A stage. All structural inputs stay earlier.
	x.reject(x.insert(x.base.ID, [3]float64{-10, -10, 10}, 2), consumer.ID)
	cross := x.insert(x.base.ID, [3]float64{10, -10, 10}, 1)
	cross.Feature.BodyID = bodyB
	x.reject(cross, "specified Body")
	if !slices.ContainsFunc(x.v.Part.Features, func(f workspace.Feature) bool { return f.ID == consumer.ID && f.EvaluationStatus == "" }) {
		t.Fatal("original consumer lost")
	}
}

// The target Body and a plane's source Body are separate authorization scopes.
func (x *insertionScene) face(after, target, source string, centroid [3]float64, reference bool) (workspace.FeatureSelection, string) {
	return x.faceWhere(after, target, source, reference, func(p workspace.FeatureSelection) bool {
		c := p.Selection.CreationEvidence.Centroid
		return math.Abs(c[0]-centroid[0]) < 1e-7 && math.Abs(c[1]-centroid[1]) < 1e-7 && math.Abs(c[2]-centroid[2]) < 1e-7
	})
}
func (x *insertionScene) faceWhere(after, target, source string, reference bool, match func(workspace.FeatureSelection) bool) (workspace.FeatureSelection, string) {
	x.t.Helper()
	r := workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, AfterFeatureID: after, BodyID: target}
	if reference {
		r.ReferenceBodyID = source
	}
	input, e := x.s.GetFeatureInput(x.t.Context(), x.v.Document.ID, r)
	if e != nil {
		x.t.Fatal(e)
	}
	r.GeometryKey = input.Artifact.GeometryKey
	r.Kind = "FACE"
	for local := uint64(1); local <= p6TopologyCount(x.t, input.Artifact.Topology, "faces"); local++ {
		r.LocalID = local
		p, e := x.s.GetFeatureInput(x.t.Context(), x.v.Document.ID, r)
		if e != nil {
			x.t.Fatal(e)
		}
		if match(*p.Selection) {
			return *p.Selection, input.Artifact.GeometryKey
		}
	}
	x.t.Fatal("stage face missing", after, source)
	return workspace.FeatureSelection{}, ""
}
func (x *insertionScene) faceInsertion(kind, after string) workspace.CommandRequest {
	x.t.Helper()
	point := [3]float64{-10, 0, 10}
	if kind == "SHELL" {
		point = [3]float64{0, 0, 20}
	}
	p, key := x.face(after, x.base.BodyID, x.base.BodyID, point, false)
	return x.request(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", AfterFeatureID: after, VersionID: x.v.Document.VersionID, GeometryKey: key, Feature: &workspace.Feature{Type: kind, BodyID: x.base.BodyID, Length: 2, Angle: 3, NeutralPlaneID: "datum-xy", Selections: []workspace.FeatureSelection{p}}})
}
func TestInsertFaceModifiersLifecycleThroughRouter(t *testing.T) {
	for _, kind := range []string{"DRAFT", "SHELL"} {
		t.Run(kind, func(t *testing.T) {
			x := newInsertionScene(t)
			ctx := t.Context()
			head := x.v.Document.VersionID
			before := append([]workspace.Feature(nil), x.v.Part.Features...)
			parameters := append([]modelcore.ParameterDefinition(nil), x.v.Part.Parameters...)
			r := x.faceInsertion(kind, x.base.ID)
			slot, expression := "angle", "1 deg + 2 deg"
			if kind == "SHELL" {
				slot, expression = "length", "1 mm + 1 mm"
			}
			r.ParameterExpressions = map[string]string{slot: expression}
			// The final Body and a newer source stage cannot impersonate this input.
			wrong := r
			f := *r.Feature
			f.Selections = append([]workspace.FeatureSelection(nil), f.Selections...)
			f.Selections[0].SourceFeatureID = x.chamfer.ID
			wrong.Feature = &f
			x.reject(wrong, "insertion stage")
			wrong = r
			wrong.GeometryKey = activeBodyArtifact(t, x.v).GeometryKey
			x.reject(wrong, "GeometryKey")
			history, e := x.s.ListHistory(ctx, x.v.Document.ID)
			if e != nil {
				t.Fatal(e)
			}
			preview, e := x.s.PreviewCommand(ctx, x.v.Document.ID, r)
			if e != nil || preview.Artifact == nil {
				t.Fatal("preview", e)
			}
			current, e := x.s.GetDocument(ctx, x.v.Document.ID, p6Actor)
			if e != nil || current.Document.VersionID != head {
				t.Fatal("preview wrote Head", e)
			}
			coldPreview, e := x.s.PreviewCommand(workspace.WithColdEvaluation(ctx), x.v.Document.ID, r)
			if e != nil || coldPreview.Artifact == nil {
				t.Fatal("cold preview", e)
			}
			for _, role := range []string{"BREP", "NAMING"} {
				if preview.Artifact.Representations[role].Digest != coldPreview.Artifact.Representations[role].Digest {
					t.Fatal("hot/cold preview mismatch", role)
				}
			}
			stale := r
			bad := *r.Feature
			bad.Length = 3
			bad.Angle = 4
			stale.Feature = &bad
			stale.ParameterExpressions = nil
			stale.PreviewID = preview.PreviewID
			if _, e = x.s.ApplyCommand(ctx, x.v.Document.ID, stale); e == nil {
				t.Fatal("changed inputs adopted old preview")
			}
			r.PreviewID = preview.PreviewID
			x.apply(r)
			created := x.v.Part.Features[2]
			if created.Type != kind || created.ID == x.chamfer.ID || created.Selections[0].SourceFeatureID != x.base.ID || x.v.Part.Features[3].ID != x.chamfer.ID {
				t.Fatal("identity or order lost", created)
			}
			for i, old := range before {
				now := x.v.Part.Features[i+func() int {
					if i > 1 {
						return 1
					}
					return 0
				}()]
				old.Order = now.Order
				if !reflect.DeepEqual(old, now) {
					t.Fatal("existing definition changed", old.Name)
				}
			}
			for _, old := range parameters {
				if !slices.ContainsFunc(x.v.Part.Parameters, func(p modelcore.ParameterDefinition) bool { return reflect.DeepEqual(old, p) }) {
					t.Fatal("old parameter changed", old.ParameterID)
				}
			}
			after, e := x.s.ListHistory(ctx, x.v.Document.ID)
			if e != nil || len(after) != len(history)+1 {
				t.Fatal("insertion not one history entry", e)
			}
			final := activeBodyArtifact(t, x.v)
			if !(final.Volume > 0 && final.Volume < 8000) {
				t.Fatal("no real material change", final.Volume)
			}
			x.apply(workspace.CommandRequest{Type: "UNDO"})
			if !reflect.DeepEqual(x.v.Part.Features, before) {
				t.Fatal("Undo lost definitions")
			}
			x.apply(workspace.CommandRequest{Type: "REDO"})
			if activeBodyArtifact(t, x.v).GeometryID != final.GeometryID || x.v.Part.Features[2].ID != created.ID {
				t.Fatal("Redo lost geometry/identity")
			}
			fresh := workspace.NewWithArtifacts(x.db, x.client, x.artifacts)
			opened, e := fresh.GetDocument(ctx, x.v.Document.ID, p6Actor)
			if e != nil || !reflect.DeepEqual(opened.Part.Features, x.v.Part.Features) {
				t.Fatal("reopen", e)
			}
			edit := workspace.CommandRequest{ActorID: p6Actor, Type: "EDIT_FEATURE", TargetID: created.ID, Feature: &created, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, created.ID)}
			cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(ctx), x.v.Document.ID, edit)
			if e != nil || cold.Artifact == nil {
				t.Fatal("cold edit", e)
			}
			for _, role := range []string{"BREP", "NAMING"} {
				if final.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
					t.Fatal("reopened cold mismatch", role)
				}
			}
			if _, e = x.s.GetFeatureInput(ctx, x.v.Document.ID, workspace.FeatureInputRequest{VersionID: head, AfterFeatureID: x.base.ID, BodyID: x.base.BodyID}); e == nil {
				t.Fatal("stale input accepted")
			}
			// A direct commit must perform the same final evaluation as preview adoption.
			x.apply(workspace.CommandRequest{Type: "UNDO"})
			r.RequestID += "-direct"
			r.PreviewID = ""
			r.VersionID = x.v.Document.VersionID
			for i := range r.Feature.Selections {
				r.Feature.Selections[i].SourceVersionID = r.VersionID
			}
			x.apply(r)
			if math.Abs(activeBodyArtifact(t, x.v).Volume-final.Volume) > 1e-6 {
				t.Fatal("direct/preview geometry mismatch")
			}
		})
	}
}

func TestInsertDraftHistoricalNeutralPlaneThroughRouter(t *testing.T) {
	x := newInsertionScene(t)
	ctx := t.Context()
	target := x.base.BodyID
	// Create an independent reference Body, then a new target stage after it.
	x.apply(workspace.CommandRequest{Type: "CREATE_BODY"})
	other := x.v.Part.ActiveBodyID
	x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := x.v.Part.Features[len(x.v.Part.Features)-1].ID
	x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 30, Y: -10}, Second: &workspace.SketchPoint2{X: 50, Y: 10}}}})
	x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	otherBase := x.v.Part.Features[len(x.v.Part.Features)-1]
	edge := findP7TopologyPick(t, x.s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return p.SourceBodyID == other && p.CreationEvidence.Centroid == [3]float64{50, 10, 5}
	})
	x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: other, Length: .5, Selections: []workspace.FeatureSelection{{Selection: edge.Selection, SourceVersionID: edge.SourceVersionID}}}})
	reference := x.v.Part.Features[len(x.v.Part.Features)-1]
	x.apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: target})
	p := findP7TopologyPick(t, x.s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return p.SourceBodyID == target && p.CreationEvidence.Centroid == [3]float64{-10, -10, 10}
	})
	x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: target, Length: .5, Selections: []workspace.FeatureSelection{{Selection: p.Selection, SourceVersionID: p.SourceVersionID}}}})
	anchor := x.v.Part.Features[len(x.v.Part.Features)-1]
	// Later reference-Body geometry must never be exposed as this earlier plane input.
	x.apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: other})
	edge = findP7TopologyPick(t, x.s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return p.SourceBodyID == other && p.CreationEvidence.Centroid == [3]float64{30, -10, 5}
	})
	x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: other, Length: .5, Selections: []workspace.FeatureSelection{{Selection: edge.Selection, SourceVersionID: edge.SourceVersionID}}}})
	later := x.v.Part.Features[len(x.v.Part.Features)-1]
	x.apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: target})
	face, key := x.faceWhere(anchor.ID, target, target, false, func(p workspace.FeatureSelection) bool {
		return p.Selection.CreationEvidence.GeometryType == "PLANE" && math.Abs(p.Selection.CreationEvidence.Centroid[0]+10) < 1e-7
	})
	neutral, referenceKey := x.faceWhere(anchor.ID, target, other, true, func(p workspace.FeatureSelection) bool {
		return math.Abs(p.Selection.CreationEvidence.Centroid[2]) < 1e-7
	})
	if neutral.SourceFeatureID != reference.ID || neutral.Selection.Anchor.FeatureID != otherBase.ID || neutral.SourceFeatureID == neutral.Selection.Anchor.FeatureID {
		t.Fatal("semantic anchor used as source stage", neutral)
	}
	r := x.request(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", AfterFeatureID: anchor.ID, VersionID: x.v.Document.VersionID, GeometryKey: key, Feature: &workspace.Feature{Type: "DRAFT", BodyID: target, Angle: 2, NeutralPlane: &neutral, Selections: []workspace.FeatureSelection{face}}})
	for _, badStage := range []string{later.ID, otherBase.ID} {
		wrong := r
		f := *r.Feature
		n := neutral
		n.SourceFeatureID = badStage
		f.NeutralPlane = &n
		wrong.Feature = &f
		x.reject(wrong, "plane")
	}
	wrong := r
	f := *r.Feature
	n := neutral
	n.SourceVersionID = "00000000-0000-7000-8000-000000000099"
	f.NeutralPlane = &n
	wrong.Feature = &f
	x.reject(wrong, "neutral plane")
	inputRequest := workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, AfterFeatureID: anchor.ID, BodyID: target, ReferenceBodyID: other}
	display, e := x.s.GetFeatureInputGeometry(ctx, x.v.Document.ID, inputRequest)
	if e != nil || display.SourceFeatureID != reference.ID || display.Artifact.GeometryKey != referenceKey || display.Artifact.BodyID != other {
		t.Fatal("reference display authorization", e)
	}
	finalOther := ""
	for _, b := range x.v.Part.Bodies {
		if b.ID == other {
			finalOther = b.GeometryKey
		}
	}
	if finalOther == referenceKey {
		t.Fatal("test has no later reference geometry")
	}
	for _, kind := range []string{"FACE", "EDGE"} {
		bad := inputRequest
		bad.Kind = kind
		bad.LocalID = 1
		bad.GeometryKey = finalOther
		if _, e = x.s.GetFeatureInput(ctx, x.v.Document.ID, bad); e == nil {
			t.Fatal("downstream/edge plane pick accepted")
		}
	}
	// A Body created after the insertion point cannot be borrowed for a plane.
	if _, e = x.s.GetFeatureInput(ctx, x.v.Document.ID, workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, AfterFeatureID: x.base.ID, BodyID: target, ReferenceBodyID: other}); e == nil {
		t.Fatal("future Body authorized")
	}
	head := x.v.Document.VersionID
	pre, e := x.s.PreviewCommand(ctx, x.v.Document.ID, r)
	if e != nil || pre.Artifact == nil {
		t.Fatal("cross-Body plane preview", e)
	}
	originalOther := finalOther
	r.PreviewID = pre.PreviewID
	x.apply(r)
	created := x.v.Part.Features[slices.IndexFunc(x.v.Part.Features, func(f workspace.Feature) bool { return f.ID == anchor.ID })+1]
	if created.NeutralPlane.SourceFeatureID != reference.ID || created.NeutralPlane.SourceVersionID != head || created.NeutralPlane.Selection.SourceBodyID != other {
		t.Fatal("plane source changed")
	}
	for _, b := range x.v.Part.Bodies {
		if b.ID == other && b.GeometryKey != originalOther {
			t.Fatal("unrelated Body changed")
		}
	}
	cold, e := x.s.PreviewCommand(workspace.WithColdEvaluation(ctx), x.v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, Type: "EDIT_FEATURE", TargetID: created.ID, Feature: &created, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, created.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold cross-Body plane", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if cold.Artifact.Representations[role].Digest != pre.Artifact.Representations[role].Digest {
			t.Fatal("plane hot/cold mismatch", role)
		}
	}
}

func TestInsertDraftDatumOrderAndFaceModifierFailureThroughRouter(t *testing.T) {
	x := newInsertionScene(t)
	top := findP7TopologyPick(t, x.s, x.v, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return p.CreationEvidence.GeometryType == "PLANE" && math.Abs(p.CreationEvidence.Centroid[2]-20) < 1e-7
	})
	x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: workspace.DatumReference{Kind: "TOPOLOGY", FeatureID: x.chamfer.ID, SourceVersionID: top.SourceVersionID, Selection: &top.Selection}}})
	future := x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1]
	x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: workspace.DatumReference{Kind: "PLANE", EntityID: future.ID}}})
	chain := x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1]
	r := x.faceInsertion("DRAFT", x.base.ID)
	r.Feature.NeutralPlaneID = chain.ID
	x.reject(r, "DEPENDENCY_CYCLE")
	r = x.faceInsertion("SHELL", x.base.ID)
	r.Feature.Length = 40
	x.reject(r, "Shell.1")
	// A datum created later in wall-clock time is legal when its full dependency
	// chain comes from standard datums, rather than a later solid stage.
	x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, DatumDefinition: &workspace.DatumTransform{Source: workspace.DatumReference{Kind: "PLANE", EntityID: "datum-xy"}}})
	r = x.faceInsertion("DRAFT", x.base.ID)
	r.Feature.NeutralPlaneID = x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1].ID
	if p, e := x.s.PreviewCommand(t.Context(), x.v.Document.ID, r); e != nil || p.Artifact == nil {
		t.Fatal("legal custom datum rejected", e)
	}
}

func TestInsertFaceModifiersBooleanAndRemovalProtectionThroughRouter(t *testing.T) {
	t.Run("Boolean output", func(t *testing.T) {
		x := newInsertionScene(t)
		body := x.base.BodyID
		x.apply(workspace.CommandRequest{Type: "CREATE_BODY"})
		toolBody := x.v.Part.ActiveBodyID
		x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		sketch := x.v.Part.Features[len(x.v.Part.Features)-1].ID
		x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 8, Y: -20}, Second: &workspace.SketchPoint2{X: 12, Y: 20}}}})
		x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 25})
		tool := x.v.Part.Features[len(x.v.Part.Features)-1]
		x.apply(workspace.CommandRequest{Type: "CREATE_BOOLEAN_FEATURE", BodyID: body, Operation: "REMOVE", KeepTools: true, Tools: []workspace.FeatureStageRef{{BodyID: toolBody, FeatureID: tool.ID}}})
		anchor := x.v.Part.Features[len(x.v.Part.Features)-1]
		top, key := x.faceWhere(anchor.ID, body, body, false, func(p workspace.FeatureSelection) bool {
			return math.Abs(p.Selection.CreationEvidence.Centroid[2]-20) < 1e-7
		})
		r := x.request(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", AfterFeatureID: anchor.ID, VersionID: x.v.Document.VersionID, GeometryKey: key, Feature: &workspace.Feature{Type: "SHELL", BodyID: body, Length: 2, Selections: []workspace.FeatureSelection{top}}})
		x.apply(r)
		last := x.v.Part.Features[len(x.v.Part.Features)-1]
		if last.Type != "SHELL" || last.Selections[0].SourceFeatureID != anchor.ID {
			t.Fatal("Boolean output stage lost")
		}
	})
	t.Run("net removal range", func(t *testing.T) {
		x := newInsertionScene(t)
		body := x.base.BodyID
		x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		sketch := x.v.Part.Features[len(x.v.Part.Features)-1].ID
		x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -3, Y: -3}, Second: &workspace.SketchPoint2{X: 3, Y: 3}}}})
		x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Length: 10})
		cut := x.v.Part.Features[len(x.v.Part.Features)-1]
		side, _ := x.face(cut.ID, body, body, [3]float64{-3, 0, 5}, false)
		x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "DRAFT", BodyID: body, Angle: 2, NeutralPlaneID: "datum-xy", Selections: []workspace.FeatureSelection{side}}})
		end := x.v.Part.Features[len(x.v.Part.Features)-1]
		x.apply(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &workspace.Feature{Type: "SOLID_PATTERN", BodyID: body, Operation: "REMOVE", Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Direction: [3]float64{1, 0, 0}, Count: 2, Spacing: 8}, SourceKind: "FEATURE_DELTA", StartFeatureID: cut.ID, Source: workspace.FeatureStageRef{BodyID: body, FeatureID: end.ID}, ResultMode: "COMBINE"}}})
		for _, kind := range []string{"DRAFT", "SHELL"} {
			pick, key := x.faceWhere(cut.ID, body, body, false, func(p workspace.FeatureSelection) bool {
				if kind == "SHELL" {
					return math.Abs(p.Selection.CreationEvidence.Centroid[2]-20) < 1e-7
				}
				return math.Abs(p.Selection.CreationEvidence.Centroid[0]+10) < 1e-7
			})
			r := x.request(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", AfterFeatureID: cut.ID, VersionID: x.v.Document.VersionID, GeometryKey: key, Feature: &workspace.Feature{Type: kind, BodyID: body, Length: 1, Angle: 2, NeutralPlaneID: "datum-xy", Selections: []workspace.FeatureSelection{pick}}})
			x.reject(r, "FEATURE_DELTA")
		}
	})
}

func TestInsertContextVariantModifiersThroughRouter(t *testing.T) {
	for _, kind := range []string{"DRAFT", "SHELL"} {
		for _, adopt := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/adopt=%v", kind, adopt), func(t *testing.T) {
				x := newInsertionScene(t)
				ctx := t.Context()
				body := x.base.BodyID
				param := ""
				for _, p := range x.v.Part.Parameters {
					if p.OwnerFeatureID == x.base.ID && p.PropertySlot == "length" {
						param = p.ParameterID
					}
				}
				x.apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: param, Value: 10, Unit: "mm"})
				x.apply(workspace.CommandRequest{Type: "CREATE_BODY"})
				referenceBody := x.v.Part.ActiveBodyID
				x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
				sketch := x.v.Part.Features[len(x.v.Part.Features)-1].ID
				x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 30, Y: -10}, Second: &workspace.SketchPoint2{X: 50, Y: 10}}}})
				x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
				reference := x.v.Part.Features[len(x.v.Part.Features)-1]
				referenceParam := ""
				for _, p := range x.v.Part.Parameters {
					if p.OwnerFeatureID == reference.ID && p.PropertySlot == "length" {
						referenceParam = p.ParameterID
					}
				}
				x.apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: body})
				anchor := x.base
				if kind == "DRAFT" {
					edge := findP7TopologyPick(t, x.s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
						return p.SourceBodyID == body && p.CreationEvidence.Centroid == [3]float64{-10, -10, 5}
					})
					x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: body, Length: .5, Selections: []workspace.FeatureSelection{{Selection: edge.Selection, SourceVersionID: edge.SourceVersionID}}}})
					anchor = x.v.Part.Features[len(x.v.Part.Features)-1]
				}

				create := func(typ, name string) workspace.DocumentView {
					v, e := x.s.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: typ, Name: name})
					if e != nil {
						t.Fatal(e)
					}
					return v
				}
				run := func(v *workspace.DocumentView, r workspace.CommandRequest) {
					r = x.request(r)
					n, e := x.s.ApplyCommand(ctx, v.Document.ID, r)
					if e != nil {
						t.Fatal(r.Type, e)
					}
					*v = n
				}
				source := create("PART", "Context insertion layout")
				run(&source, workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "height", Value: 20, Unit: "mm"})
				run(&source, workspace.CommandRequest{Type: "CREATE_PUBLICATION", Name: "Height", PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: source.Part.Parameters[0].ParameterID})
				root := create("PRODUCT", "Projected historical insertion")
				run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: source.Document.ID})
				run(&root, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: x.v.Document.ID})
				for _, parameter := range []string{param, referenceParam} {
					a, e := x.s.GetProductDesignSession(ctx, root.Document.ID, root.Product.Instances[0].ID)
					if e != nil {
						t.Fatal(e)
					}
					b, e := x.s.GetProductDesignSession(ctx, root.Document.ID, root.Product.Instances[1].ID)
					if e != nil {
						t.Fatal(e)
					}
					root, e = x.s.CreateProductContextBinding(ctx, root.Document.ID, workspace.ProductContextBindingRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-input-%s", root.Document.ID, parameter), OwningInstancePath: *b.ActiveInstancePath, SourceInstancePath: *a.ActiveInstancePath, PublicationID: source.Part.Publications[0].ID, PublicationType: "PARAMETER", TargetKind: "PARAMETER", TargetID: parameter})
					if e != nil {
						t.Fatal(e)
					}
				}
				acceptedRoot := root.Document.VersionID
				run(&source, workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: source.Part.Parameters[0].ParameterID, Value: 25, Unit: "mm"})
				pendingRoot, pendingError := x.s.GetDocument(ctx, root.Document.ID, p6Actor)
				if pendingError != nil || pendingRoot.Document.VersionID != acceptedRoot {
					t.Fatal("unaccepted source update advanced Product", pendingError)
				}
				root = pendingRoot
				for _, feature := range root.ContextVariants[0].Part.Features {
					if (feature.ID == x.base.ID || feature.ID == reference.ID) && feature.Length != 20 {
						t.Fatal("projection read source Head instead of accepted 20 mm", feature.Length)
					}
				}

				var e error
				x.v, e = x.s.GetDocument(ctx, x.v.Document.ID, p6Actor)
				if e != nil {
					t.Fatal(e)
				}
				variant := root.ContextVariants[0]
				scope := workspace.ContextReadScope{RootProductDocumentID: root.Document.ID, RootProductRevisionID: root.Document.VersionID, ContextVariantID: variant.VariantKey, InstancePath: &variant.OwningInstancePath}
				pick := func(other string, match func(modelcore.TopologySelectionEvidence) bool) (workspace.FeatureSelection, string) {
					r := workspace.FeatureInputRequest{ContextReadScope: scope, VersionID: x.v.Document.VersionID, AfterFeatureID: anchor.ID, BodyID: body, ReferenceBodyID: other}
					input, e := x.s.GetFeatureInput(ctx, x.v.Document.ID, r)
					if e != nil {
						t.Fatal(e)
					}
					r.GeometryKey = input.Artifact.GeometryKey
					r.Kind = "FACE"
					for i := uint64(1); i <= p6TopologyCount(t, input.Artifact.Topology, "faces"); i++ {
						r.LocalID = i
						p, e := x.s.GetFeatureInput(ctx, x.v.Document.ID, r)
						if e != nil {
							t.Fatal(e)
						}
						if match(p.Selection.Selection.CreationEvidence) {
							return *p.Selection, r.GeometryKey
						}
					}
					t.Fatal("context stage face missing")
					return workspace.FeatureSelection{}, ""
				}
				face, key := pick("", func(e modelcore.TopologySelectionEvidence) bool {
					if kind == "SHELL" {
						return math.Abs(e.Centroid[2]-10) < 1e-7
					}
					return math.Abs(e.Centroid[0]+10) < 1e-7 && e.GeometryType == "PLANE"
				})
				independent := x.input(anchor.ID, body)
				if independent.Artifact.GeometryKey == key {
					t.Fatal("fixture did not project 10 to 20 mm")
				}
				f := workspace.Feature{Type: kind, BodyID: body, Length: 2, Angle: 2, Selections: []workspace.FeatureSelection{face}}
				if kind == "DRAFT" {
					neutral, _ := pick(referenceBody, func(e modelcore.TopologySelectionEvidence) bool { return math.Abs(e.Centroid[2]-10) < 1e-7 })
					f.NeutralPlane = &neutral
					if neutral.SourceFeatureID != reference.ID {
						t.Fatal("reference stage lost")
					}
				}
				r := x.request(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", AfterFeatureID: anchor.ID, VersionID: x.v.Document.VersionID, GeometryKey: key, Feature: &f, RootProductDocumentID: scope.RootProductDocumentID, RootProductRevisionID: scope.RootProductRevisionID, ContextVariantID: scope.ContextVariantID, InstancePath: scope.InstancePath})

				wrong := r
				wrong.GeometryKey = independent.Artifact.GeometryKey
				x.reject(wrong, "GeometryKey")
				wrong = r
				forged := f
				forged.Selections = append([]workspace.FeatureSelection(nil), f.Selections...)
				forged.Selections[0].Selection.CreationEvidence.Centroid[2] += 10
				wrong.Feature = &forged
				x.reject(wrong, "independent definition evidence")
				wrong = r
				wrong.RootProductRevisionID = source.Document.VersionID
				x.reject(wrong, "CONTEXT_PRODUCT_REVISION_CHANGED")
				wrong = r
				wrong.ContextVariantID = "stale-variant"
				x.reject(wrong, "CONTEXT_VARIANT_CHANGED")
				if f.NeutralPlane != nil {
					wrong = r
					badFeature := f
					badPlane := *f.NeutralPlane
					badPlane.Selection.CreationEvidence.Centroid[2] += 10
					badFeature.NeutralPlane = &badPlane
					wrong.Feature = &badFeature
					x.reject(wrong, "independent definition evidence")
				}

				preview, e := x.s.PreviewCommand(ctx, x.v.Document.ID, r)
				if e != nil || preview.Artifact == nil {
					t.Fatal("projected preview", e)
				}
				cold, e := x.s.PreviewCommand(workspace.WithColdEvaluation(ctx), x.v.Document.ID, r)
				if e != nil || cold.Artifact == nil {
					t.Fatal("cold projected preview", e)
				}
				for _, role := range []string{"BREP", "NAMING"} {
					if preview.Artifact.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
						t.Fatal("projected hot/cold mismatch", role)
					}
				}
				unchanged, e := x.s.GetDocument(ctx, x.v.Document.ID, p6Actor)
				if e != nil || unchanged.Document.VersionID != x.v.Document.VersionID {
					t.Fatal("preview wrote Head", e)
				}
				if adopt {
					r.PreviewID = preview.PreviewID
				}
				x.apply(r)
				created := x.v.Part.Features[slices.IndexFunc(x.v.Part.Features, func(f workspace.Feature) bool { return f.ID == anchor.ID })+1]
				if !reflect.DeepEqual(created.Selections, f.Selections) || !reflect.DeepEqual(created.NeutralPlane, f.NeutralPlane) {
					t.Fatal("projected topology/evidence was persisted")
				}
				for _, p := range x.v.Part.Parameters {
					if (p.ParameterID == param || p.ParameterID == referenceParam) && math.Abs(p.EvaluatedValue.SIValue-.01) > 1e-9 {
						t.Fatal("projected value overwrote default")
					}
				}
				if _, e = x.s.GetFeatureInput(ctx, x.v.Document.ID, workspace.FeatureInputRequest{ContextReadScope: scope, VersionID: x.v.Document.VersionID, AfterFeatureID: anchor.ID, BodyID: body}); e == nil {
					t.Fatal("old occurrence accepted after commit")
				}
			})
		}
	}
}
