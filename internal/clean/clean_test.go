package clean

import (
	"os"
	"path/filepath"
	"testing"
)

// result creates a fake output folder at dir.
func result(t *testing.T, dir string) {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, "RPMS", "x86_64"), 0o755)
	os.WriteFile(filepath.Join(dir, "RPMS", "x86_64", "kernel-1.rpm"), []byte("rpm"), 0o644)
	for _, f := range []string{"build.log", "manifest.txt", "toolchain.txt", "SHA256SUMS"} {
		os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644)
	}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func TestDirSingleResult(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out", "original")
	result(t, out)

	rep, err := Dir(out, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Removed) != 5 || rep.Bytes != 3+4 {
		t.Errorf("removed %v (%d bytes)", rep.Removed, rep.Bytes)
	}
	if exists(out) {
		t.Error("empty result folder not removed")
	}
	if !exists(filepath.Dir(out)) {
		t.Error("parent of the target was removed")
	}
}

func TestDirTreeKeepsUnknownFiles(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	result(t, filepath.Join(out, "original"))
	result(t, filepath.Join(out, "patched"))
	os.WriteFile(filepath.Join(out, "patched", "notes.txt"), []byte("mine"), 0o644)
	os.MkdirAll(filepath.Join(out, "empty"), 0o755)

	rep, err := Dir(out, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Removed) != 10 {
		t.Errorf("removed %d artifacts, want 10", len(rep.Removed))
	}
	if len(rep.Kept) != 1 || rep.Kept[0] != filepath.Join(out, "patched", "notes.txt") {
		t.Errorf("kept = %v", rep.Kept)
	}
	if exists(filepath.Join(out, "original")) || exists(filepath.Join(out, "empty")) {
		t.Error("empty folders not removed")
	}
	if !exists(filepath.Join(out, "patched", "notes.txt")) {
		t.Error("unknown file was deleted")
	}
	if exists(filepath.Join(out, "patched", "RPMS")) {
		t.Error("artifact left behind")
	}
}

func TestDirNoResultsTouchesNothing(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "important.rpm"), []byte("x"), 0o644)
	rep, err := Dir(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Removed) != 0 || !exists(filepath.Join(dir, "important.rpm")) {
		t.Errorf("removed %v from a folder without build results", rep.Removed)
	}
}

func TestDirDryRun(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	result(t, filepath.Join(out, "original"))

	rep, err := Dir(out, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Removed) != 5 || len(rep.Dirs) != 2 {
		t.Errorf("dry run reported removed=%v dirs=%v", rep.Removed, rep.Dirs)
	}
	if !exists(filepath.Join(out, "original", "RPMS", "x86_64", "kernel-1.rpm")) {
		t.Error("dry run deleted files")
	}
}

func TestDirErrors(t *testing.T) {
	if _, err := Dir(filepath.Join(t.TempDir(), "missing"), false); err == nil {
		t.Error("missing target: no error")
	}
	f := filepath.Join(t.TempDir(), "file")
	os.WriteFile(f, nil, 0o644)
	if _, err := Dir(f, false); err == nil {
		t.Error("file target: no error")
	}
}
