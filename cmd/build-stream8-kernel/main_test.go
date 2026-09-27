package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestParseArgsFlagsAnywhere(t *testing.T) {
	o, err := parseArgs([]string{"--jobs", "4", "in.src.rpm", "out", "--force"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if o.src != "in.src.rpm" || o.out != "out" || o.jobs != 4 || !o.force {
		t.Errorf("parsed %+v", o)
	}
}

func TestParseArgsErrors(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"only-one"},
		{"a", "b", "c"},
		{"--jobs", "-1", "a", "b"},
		{"--unknown", "a", "b"},
	} {
		if _, err := parseArgs(args, io.Discard); err == nil {
			t.Errorf("parseArgs(%q) succeeded, want error", args)
		}
	}
}

func TestPrepareOutDirRefusesResults(t *testing.T) {
	dir := t.TempDir()
	if _, err := prepareOutDir(filepath.Join(dir, "new", "sub"), false); err != nil {
		t.Errorf("new dir: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "build.log"), nil, 0o644)
	_, err := prepareOutDir(dir, false)
	if exitCode(err) != exitUsage {
		t.Errorf("existing results: err = %v (code %d), want exit %d", err, exitCode(err), exitUsage)
	}
	if _, err := prepareOutDir(dir, true); err != nil {
		t.Errorf("--force: %v", err)
	}
}

func TestContainerErrorCodes(t *testing.T) {
	for code, want := range map[int]int{
		2: exitInput, 3: exitBuildDeps, 4: exitRPMBuild, 5: exitExport,
		125: exitDocker, 126: exitDocker, 127: exitDocker, 1: exitRPMBuild,
	} {
		if got := exitCode(containerError(code, t.TempDir())); got != want {
			t.Errorf("container exit %d -> %d, want %d", code, got, want)
		}
	}
}

func TestTail(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log")
	os.WriteFile(p, []byte("1\n2\n3\n4\n"), 0o644)
	if got := tail(p, 2); got != "  3\n  4" {
		t.Errorf("tail = %q", got)
	}
}

func TestParseArgsClean(t *testing.T) {
	o, err := parseArgs([]string{"--clean"}, io.Discard)
	if err != nil || !o.clean || len(o.cleanDirs) != 1 || o.cleanDirs[0] != defaultCleanDir {
		t.Errorf("--clean: %+v, %v", o, err)
	}
	o, err = parseArgs([]string{"--clean", "out/original", "out/patched", "--dry-run"}, io.Discard)
	if err != nil || !o.dryRun || len(o.cleanDirs) != 2 || o.cleanDirs[1] != "out/patched" {
		t.Errorf("--clean dirs: %+v, %v", o, err)
	}
	if _, err := parseArgs([]string{"--dry-run", "a", "b"}, io.Discard); err == nil {
		t.Error("--dry-run without --clean accepted")
	}
}

func TestRunClean(t *testing.T) {
	root := t.TempDir()
	res := filepath.Join(root, "out", "original")
	os.MkdirAll(filepath.Join(res, "RPMS", "x86_64"), 0o755)
	os.WriteFile(filepath.Join(res, "build.log"), []byte("log"), 0o644)

	o := &options{clean: true, cleanDirs: []string{res, filepath.Join(root, "missing")}}
	if err := runClean(o, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(res); !os.IsNotExist(err) {
		t.Error("result folder still exists")
	}
}
