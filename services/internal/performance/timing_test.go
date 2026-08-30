package performance

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRecorderAggregatesBoundedPhaseNames(t *testing.T) {
	ctx, recorder := WithRecorder(context.Background())
	finish := Start(ctx, "Geometry Evaluate")
	time.Sleep(time.Millisecond)
	finish()
	header := recorder.Header()
	if !strings.Contains(header, "geometry-evaluate;dur=") {
		t.Fatalf("unexpected Server-Timing header %q", header)
	}
	if recorder.SnapshotMilliseconds()["Geometry Evaluate"] <= 0 {
		t.Fatal("phase duration was not recorded")
	}
}
