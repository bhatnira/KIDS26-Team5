package agent

import (
	"testing"

	"antelope/internal/modules/agent/daytona"
)

func TestRestoreArtifactPath(t *testing.T) {
	cases := []struct {
		name string
		want string
		ok   bool
	}{
		{"out/result.png", "out/result.png", true},
		{"work/inputs/data.csv", "work/inputs/data.csv", true},
		{"runs/run_1/log.txt", "runs/run_1/log.txt", true},
		{"inline/123_fig.png", "", false},   // never lived on the workspace fs
		{"out", "", false},                  // a file named "out" must not shadow the dir
		{"output/x.txt", "", false},         // prefix match must be path-segment exact
		{"/out/result.png", "", false},      // absolute
		{"out/../../etc/passwd", "", false}, // traversal
		{"out//x.png", "", false},           // non-clean
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := restoreArtifactPath(c.name)
		if got != c.want || ok != c.ok {
			t.Errorf("restoreArtifactPath(%q) = (%q, %v), want (%q, %v)",
				c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestSessionArtifactSpecs(t *testing.T) {
	arts := []SessionArtifactObject{
		{Name: "out/small.csv", Bucket: "b", Key: "agent/sid/out/small.csv/3", Version: 3, Size: 100},
		{Name: "out/huge.h5ad", Bucket: "b", Key: "agent/sid/out/huge.h5ad/0", Version: 0, Size: sdkUploadMaxBytes + 1},
		{Name: "inline/123_fig.png", Bucket: "b", Key: "agent/sid/inline/123_fig.png/0", Version: 0, Size: 10},
		{Name: "out/nokey.txt", Bucket: "b", Key: "", Version: 0, Size: 10},
	}
	specs := sessionArtifactSpecs(arts)
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d: %+v", len(specs), specs)
	}
	if specs[0].From != daytona.SchemeS3Buf+"b/agent/sid/out/small.csv/3" {
		t.Errorf("small artifact should use buffered scheme, got %q", specs[0].From)
	}
	if specs[0].To != "out/small.csv" || !specs[0].Pin {
		t.Errorf("unexpected spec for small artifact: %+v", specs[0])
	}
	if specs[1].From != daytona.SchemeS3URL+"b/agent/sid/out/huge.h5ad/0" {
		t.Errorf("large artifact should use presigned scheme, got %q", specs[1].From)
	}
}

func TestAttachmentInputSpecsHonorsStagedPath(t *testing.T) {
	atts := []AttachmentRef{
		{Bucket: "b", Key: "u/1/data.csv", Name: "data.csv", Size: 10, StagedPath: "work/inputs/data_2.csv"},
		{Bucket: "b", Key: "u/2/other.csv", Name: "other.csv", Size: 10},
		{Bucket: "b", Key: "u/3/gone.csv", Name: "gone.csv", Unavailable: true},
	}
	specs := attachmentInputSpecs(atts)
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d: %+v", len(specs), specs)
	}
	if specs[0].To != "work/inputs/data_2.csv" {
		t.Errorf("pinned StagedPath ignored, got To=%q", specs[0].To)
	}
	if specs[1].To != "work/inputs/other.csv" {
		t.Errorf("fallback staged path wrong, got To=%q", specs[1].To)
	}
}
