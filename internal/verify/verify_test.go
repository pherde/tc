package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeOut creates an output folder with the given RPMs and a matching
// SHA256SUMS.
func makeOut(t *testing.T, names ...string) string {
	t.Helper()
	out := t.TempDir()
	os.MkdirAll(filepath.Join(out, "RPMS", "x86_64"), 0o755)
	var sums strings.Builder
	for _, n := range names {
		rel := filepath.Join("RPMS", "x86_64", n)
		os.WriteFile(filepath.Join(out, rel), []byte(n), 0o644)
		h := sha256.Sum256([]byte(n))
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(h[:]), rel)
	}
	os.WriteFile(filepath.Join(out, "SHA256SUMS"), []byte(sums.String()), 0o644)
	os.WriteFile(filepath.Join(out, "manifest.txt"), []byte("nvr:            kernel-4.18.0-448.el8\nbuild_seconds:  10\n"), 0o644)
	return out
}

var full = []string{
	"kernel-4.18.0-448.el8.x86_64.rpm",
	"kernel-core-4.18.0-448.el8.x86_64.rpm",
	"kernel-modules-4.18.0-448.el8.x86_64.rpm",
	"kernel-modules-extra-4.18.0-448.el8.x86_64.rpm",
}

func TestCheckOK(t *testing.T) {
	res, err := Check(makeOut(t, full...))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RPMs) != len(full) {
		t.Errorf("RPMs = %v", res.RPMs)
	}
	if res.Manifest["nvr"] != "kernel-4.18.0-448.el8" {
		t.Errorf("manifest nvr = %q", res.Manifest["nvr"])
	}
}

func TestCheckCorrupted(t *testing.T) {
	out := makeOut(t, full...)
	os.WriteFile(filepath.Join(out, "RPMS", "x86_64", full[1]), []byte("tampered"), 0o644)
	if _, err := Check(out); !errors.Is(err, ErrVerify) {
		t.Errorf("err = %v, want ErrVerify", err)
	}
}

func TestCheckUnlistedRPM(t *testing.T) {
	out := makeOut(t, full...)
	os.WriteFile(filepath.Join(out, "RPMS", "x86_64", "extra-1.rpm"), []byte("x"), 0o644)
	if _, err := Check(out); !errors.Is(err, ErrVerify) {
		t.Errorf("err = %v, want ErrVerify", err)
	}
}

func TestCheckMissingRequired(t *testing.T) {
	// kernel-modules-extra must not satisfy kernel-modules.
	out := makeOut(t, full[0], full[1], full[3])
	_, err := Check(out)
	if !errors.Is(err, ErrVerify) || !strings.Contains(err.Error(), "kernel-modules ") {
		t.Errorf("err = %v, want missing kernel-modules", err)
	}
}

func TestCheckNoSums(t *testing.T) {
	if _, err := Check(t.TempDir()); !errors.Is(err, ErrVerify) {
		t.Errorf("err = %v, want ErrVerify", err)
	}
}
