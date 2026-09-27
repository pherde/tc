package buildenv

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestImageRef(t *testing.T) {
	if Hash() != Hash() {
		t.Fatal("hash is not stable")
	}
	if !regexp.MustCompile(`^stream8-kernel-builder:[0-9a-f]{12}$`).MatchString(ImageRef()) {
		t.Errorf("ImageRef() = %s", ImageRef())
	}
}

func TestExtract(t *testing.T) {
	dir := t.TempDir()
	if err := Extract(dir); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		got, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		want, _ := os.ReadFile(n) // the source file next to this test
		if string(got) != string(want) {
			t.Errorf("%s differs from the embedded copy", n)
		}
	}
	if fi, _ := os.Stat(filepath.Join(dir, "build.sh")); fi.Mode().Perm()&0o100 == 0 {
		t.Error("build.sh is not executable")
	}
}
