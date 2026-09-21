package stage

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kevin-hanselman/dud/src/artifact"
	"github.com/pkg/errors"
)

func newImportStage() Stage {
	return Stage{
		// FromFile/FromReader clean an empty working dir to ".".
		WorkingDir: ".",
		Import: &ImportSpec{
			Repo:    "git@example.com:org/registry.git",
			Rev:     "v1",
			RevLock: "6c73875a5f5b522f90b5afa9ab12585f64327ca7",
			Path:    "data/raw.csv",
			Remote:  "s3:registry",
		},
		Outputs: map[string]*artifact.Artifact{
			"raw.csv": {Path: "raw.csv", Checksum: "abc"},
		},
	}
}

func TestImportRoundTrip(t *testing.T) {
	stg := newImportStage()
	checksum, err := stg.CalculateChecksum()
	if err != nil {
		t.Fatal(err)
	}
	stg.Checksum = checksum

	buf := new(bytes.Buffer)
	if err := stg.Serialize(buf); err != nil {
		t.Fatal(err)
	}

	// Serialize must not alias the caller's ImportSpec.
	stg.Import.Rev = "v2"
	if strings.Contains(buf.String(), "v2") {
		t.Fatal("Serialize aliased the ImportSpec")
	}
	stg.Import.Rev = "v1"

	got, err := FromReader(buf, "raw.csv.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// toFileFormat blanks artifact paths on the shared pointers, so compare
	// against a fresh copy rather than stg.
	want := newImportStage()
	want.Checksum = checksum
	want.Inputs = map[string]*artifact.Artifact{}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("Stage -want +got:\n%s", diff)
	}
}

func TestFromReaderMinimalImport(t *testing.T) {
	// A hand-written import stage may carry only the user-facing fields;
	// `dud update` fills in the rest.
	yamlText := `import:
  repo: ../registry
  path: ./data//raw.csv
outputs:
  raw.csv:
`
	got, err := FromReader(strings.NewReader(yamlText), "raw.csv.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := Stage{
		WorkingDir: ".",
		Import:     &ImportSpec{Repo: "../registry", Path: "data/raw.csv"},
		Inputs:     map[string]*artifact.Artifact{},
		Outputs: map[string]*artifact.Artifact{
			"raw.csv": {Path: "raw.csv"},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("Stage -want +got:\n%s", diff)
	}
}

func TestFromReaderRejectsUnknownFields(t *testing.T) {
	yamlText := `import:
  repo: ../registry
  path: data/raw.csv
  branch: main
outputs:
  raw.csv:
`
	if _, err := FromReader(strings.NewReader(yamlText), "raw.csv.yaml"); err == nil {
		t.Fatal("expected strict decoding to fail on unknown field")
	}
}

func TestValidateImport(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(stg *Stage)
		want   string
	}{
		{
			name:   "valid",
			mutate: func(stg *Stage) {},
			want:   "",
		},
		{
			name:   "command",
			mutate: func(stg *Stage) { stg.Command = "echo" },
			want:   "import stage cannot have a command",
		},
		{
			name: "inputs",
			mutate: func(stg *Stage) {
				stg.Inputs = map[string]*artifact.Artifact{"in": {Path: "in"}}
			},
			want: "import stage cannot have inputs",
		},
		{
			name: "two outputs",
			mutate: func(stg *Stage) {
				stg.Outputs["other"] = &artifact.Artifact{Path: "other"}
			},
			want: "import stage must have exactly one output",
		},
		{
			name:   "no repo",
			mutate: func(stg *Stage) { stg.Import.Repo = "" },
			want:   "import stage has no repo",
		},
		{
			name:   "no path",
			mutate: func(stg *Stage) { stg.Import.Path = "" },
			want:   "import stage has no path",
		},
		{
			name:   "dot path",
			mutate: func(stg *Stage) { stg.Import.Path = "." },
			want:   "import stage has no path",
		},
		{
			name:   "double dot path",
			mutate: func(stg *Stage) { stg.Import.Path = "../x" },
			want:   "import path ../x is outside of the registry root",
		},
		{
			name:   "absolute path",
			mutate: func(stg *Stage) { stg.Import.Path = "/x" },
			want:   "import path /x is an absolute path",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stg := newImportStage()
			c.mutate(&stg)
			err := errors.Cause(stg.Validate("raw.csv.yaml"))
			if c.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %q", c.want)
			}
			if diff := cmp.Diff(c.want, err.Error()); diff != "" {
				t.Fatalf("error -want +got:\n%s", diff)
			}
		})
	}
}

func TestImportFieldsAffectChecksum(t *testing.T) {
	base := newImportStage()
	baseChecksum, err := base.CalculateChecksum()
	if err != nil {
		t.Fatal(err)
	}

	mutations := map[string]func(spec *ImportSpec){
		"repo":     func(spec *ImportSpec) { spec.Repo = "other" },
		"rev":      func(spec *ImportSpec) { spec.Rev = "v2" },
		"rev-lock": func(spec *ImportSpec) { spec.RevLock = "deadbeef" },
		"path":     func(spec *ImportSpec) { spec.Path = "data/other.csv" },
		"remote":   func(spec *ImportSpec) { spec.Remote = "s3:other" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			stg := newImportStage()
			mutate(stg.Import)
			checksum, err := stg.CalculateChecksum()
			if err != nil {
				t.Fatal(err)
			}
			if checksum == baseChecksum {
				t.Fatalf("changing import %s did not change the checksum", name)
			}
		})
	}

	t.Run("output checksum does not affect checksum", func(t *testing.T) {
		stg := newImportStage()
		stg.Outputs["raw.csv"].Checksum = "different"
		checksum, err := stg.CalculateChecksum()
		if err != nil {
			t.Fatal(err)
		}
		if checksum != baseChecksum {
			t.Fatal("output checksum changed the stage checksum")
		}
	})
}
