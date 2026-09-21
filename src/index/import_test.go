package index

import (
	"os/exec"
	"testing"

	"github.com/kevin-hanselman/dud/src/agglog"
	"github.com/kevin-hanselman/dud/src/artifact"
	"github.com/kevin-hanselman/dud/src/mocks"
	"github.com/kevin-hanselman/dud/src/stage"
	"github.com/kevin-hanselman/dud/src/strategy"
	"github.com/pkg/errors"
)

// newImportIndex returns an import stage plus a downstream stage that
// consumes it, mirroring how a project would use a registry artifact.
func newImportIndex(registryRemote string) (Index, *stage.Stage, *stage.Stage) {
	imported := &stage.Stage{
		Import: &stage.ImportSpec{
			Repo:    "../registry",
			Rev:     "v1",
			RevLock: "6c73875a5f5b522f90b5afa9ab12585f64327ca7",
			Path:    "raw.csv",
			Remote:  registryRemote,
		},
		Outputs: map[string]*artifact.Artifact{
			"raw.csv": {Path: "raw.csv", Checksum: "registry_checksum"},
		},
	}
	downstream := &stage.Stage{
		Command: "cp raw.csv copy.csv",
		Inputs: map[string]*artifact.Artifact{
			"raw.csv": {Path: "raw.csv", SkipCache: true},
		},
		Outputs: map[string]*artifact.Artifact{
			"copy.csv": {Path: "copy.csv"},
		},
	}
	idx := Index{
		"raw.csv.yaml": imported,
		"copy.yaml":    downstream,
	}
	return idx, imported, downstream
}

func TestFetchImport(t *testing.T) {
	rootDir := "project/root"
	logger := agglog.NewNullLogger()

	t.Run("uses the registry remote, not the project remote", func(t *testing.T) {
		idx, imported, downstream := newImportIndex("registry:bucket")
		mockCache := mocks.Cache{}
		expectOutputsFetched(imported, &mockCache, rootDir, "registry:bucket")
		expectOutputsFetched(downstream, &mockCache, rootDir, "project:bucket")

		if err := idx.Fetch(
			"copy.yaml",
			&mockCache,
			rootDir,
			true,
			"project:bucket",
			make(map[string]bool),
			make(map[string]bool),
			logger,
		); err != nil {
			t.Fatal(err)
		}
		mockCache.AssertExpectations(t)
	})

	t.Run("no project remote is fine for an import", func(t *testing.T) {
		idx, imported, _ := newImportIndex("registry:bucket")
		mockCache := mocks.Cache{}
		expectOutputsFetched(imported, &mockCache, rootDir, "registry:bucket")

		if err := idx.Fetch(
			"raw.csv.yaml",
			&mockCache,
			rootDir,
			true,
			"",
			make(map[string]bool),
			make(map[string]bool),
			logger,
		); err != nil {
			t.Fatal(err)
		}
		mockCache.AssertExpectations(t)
	})

	t.Run("no project remote fails for a regular stage", func(t *testing.T) {
		idx, _, _ := newImportIndex("registry:bucket")
		mockCache := mocks.Cache{}

		err := idx.Fetch(
			"copy.yaml",
			&mockCache,
			rootDir,
			false,
			"",
			make(map[string]bool),
			make(map[string]bool),
			logger,
		)
		if !errors.Is(err, NoRemoteError{}) {
			t.Fatalf("got error %v, want NoRemoteError", err)
		}
		mockCache.AssertExpectations(t)
	})

	t.Run("import without a remote asks for an update", func(t *testing.T) {
		idx, _, _ := newImportIndex("")
		mockCache := mocks.Cache{}

		err := idx.Fetch(
			"raw.csv.yaml",
			&mockCache,
			rootDir,
			true,
			"project:bucket",
			make(map[string]bool),
			make(map[string]bool),
			logger,
		)
		if err == nil {
			t.Fatal("expected error")
		}
		want := "import stage raw.csv.yaml has no remote; run 'dud update raw.csv.yaml'"
		if err.Error() != want {
			t.Fatalf("got error %q, want %q", err.Error(), want)
		}
		mockCache.AssertExpectations(t)
	})
}

func TestPushSkipsImport(t *testing.T) {
	rootDir := "project/root"
	logger := agglog.NewNullLogger()
	idx, _, downstream := newImportIndex("registry:bucket")
	mockCache := mocks.Cache{}
	expectOutputsPushed(downstream, &mockCache, rootDir, "project:bucket")

	pushed := make(map[string]bool)
	if err := idx.Push(
		"copy.yaml",
		&mockCache,
		rootDir,
		true,
		"project:bucket",
		pushed,
		make(map[string]bool),
		logger,
	); err != nil {
		t.Fatal(err)
	}
	if !pushed["raw.csv.yaml"] {
		t.Fatal("import stage should be marked as pushed")
	}
	mockCache.AssertExpectations(t)
}

func TestCommitSkipsImport(t *testing.T) {
	rootDir := "project/root"
	logger := agglog.NewNullLogger()
	idx, imported, downstream := newImportIndex("registry:bucket")
	mockCache := mocks.Cache{}
	expectOutputsCommitted(downstream, &mockCache, rootDir, strategy.LinkStrategy)

	committed := make(map[string]bool)
	if err := idx.Commit(
		"copy.yaml",
		&mockCache,
		rootDir,
		strategy.LinkStrategy,
		committed,
		make(map[string]bool),
		logger,
	); err != nil {
		t.Fatal(err)
	}
	if !committed["raw.csv.yaml"] {
		t.Fatal("import stage should be marked as committed")
	}
	if imported.Outputs["raw.csv"].Checksum != "registry_checksum" {
		t.Fatal("import output checksum must not be touched by commit")
	}
	// The downstream stage records the registry checksum for its input.
	if downstream.Inputs["raw.csv"].Checksum != "registry_checksum" {
		t.Fatalf(
			"downstream input checksum = %q, want registry_checksum",
			downstream.Inputs["raw.csv"].Checksum,
		)
	}
	mockCache.AssertExpectations(t)
}

func TestCheckoutImport(t *testing.T) {
	rootDir := "project/root"
	logger := agglog.NewNullLogger()
	idx, imported, _ := newImportIndex("registry:bucket")
	mockCache := mocks.Cache{}
	expectOutputsCheckedOut(imported, &mockCache, rootDir, strategy.LinkStrategy)

	if err := idx.Checkout(
		"raw.csv.yaml",
		&mockCache,
		rootDir,
		strategy.LinkStrategy,
		true,
		make(map[string]bool),
		make(map[string]bool),
		logger,
	); err != nil {
		t.Fatal(err)
	}
	mockCache.AssertExpectations(t)
}

func TestRunImport(t *testing.T) {
	rootDir := "project/root"
	logger := agglog.NewNullLogger()
	idx, imported, _ := newImportIndex("registry:bucket")
	mockCache := mocks.Cache{}
	// An import has no command, so even an out-of-date output leads to
	// "nothing to do" rather than an attempt to run anything. The stage
	// checksum must be current, otherwise Run short-circuits on "definition
	// modified" before ever looking at the output.
	var err error
	imported.Checksum, err = imported.CalculateChecksum()
	if err != nil {
		t.Fatal(err)
	}
	art := *imported.Outputs["raw.csv"]
	mockCache.On("Status", rootDir, art, true).Return(
		artifact.Status{Artifact: art, ContentsMatch: false}, nil,
	).Once()

	runCommandOrig := runCommand
	defer func() { runCommand = runCommandOrig }()
	runCommand = func(_ *exec.Cmd) error {
		t.Fatal("no command should be run for an import stage")
		return nil
	}

	ran := make(map[string]bool)
	if err := idx.Run(
		"raw.csv.yaml",
		&mockCache,
		rootDir,
		true,
		ran,
		make(map[string]bool),
		logger,
	); err != nil {
		t.Fatal(err)
	}
	mockCache.AssertExpectations(t)
}
