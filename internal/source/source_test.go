package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

var payload = []byte("not really an rpm")

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestIsURL(t *testing.T) {
	for in, want := range map[string]bool{
		"https://vault.centos.org/x.src.rpm": true,
		"http://host/x.src.rpm":              true,
		"ftp://host/x.src.rpm":               false,
		"kernel.src.rpm":                     false,
		"/abs/kernel.src.rpm":                false,
		"https://":                           false,
	} {
		if got := IsURL(in); got != want {
			t.Errorf("IsURL(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestStageLocalCopiesAndHashes(t *testing.T) {
	src := filepath.Join(t.TempDir(), "kernel-1.src.rpm")
	os.WriteFile(src, payload, 0o644)

	s, err := Stage(context.Background(), src, sum(payload), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Cleanup()

	if filepath.Base(s.Path) != "kernel-1.src.rpm" {
		t.Errorf("staged name = %s", filepath.Base(s.Path))
	}
	a, _ := os.Stat(src)
	b, _ := os.Stat(s.Path)
	if os.SameFile(a, b) {
		t.Error("staged file is a hard link to the original; it must be a copy")
	}
	if s.SHA256 != sum(payload) {
		t.Errorf("sha = %s", s.SHA256)
	}
}

func TestStageRenamesToSrcRPM(t *testing.T) {
	src := filepath.Join(t.TempDir(), "download.bin")
	os.WriteFile(src, payload, 0o644)
	s, err := Stage(context.Background(), src, "", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Cleanup()
	if filepath.Base(s.Path) != "input.src.rpm" {
		t.Errorf("staged name = %s, want input.src.rpm", filepath.Base(s.Path))
	}
}

func TestStageLocalErrors(t *testing.T) {
	_, err := Stage(context.Background(), "/nonexistent.src.rpm", "", io.Discard)
	if !errors.Is(err, ErrInput) {
		t.Errorf("missing file: err = %v, want ErrInput", err)
	}
	_, err = Stage(context.Background(), t.TempDir(), "", io.Discard)
	if !errors.Is(err, ErrInput) {
		t.Errorf("directory: err = %v, want ErrInput", err)
	}
}

func TestStageChecksumMismatchCleansUp(t *testing.T) {
	src := filepath.Join(t.TempDir(), "k.src.rpm")
	os.WriteFile(src, payload, 0o644)
	before, _ := filepath.Glob(filepath.Join(os.TempDir(), "build-stream8-kernel-input-*"))

	_, err := Stage(context.Background(), src, sum([]byte("other")), io.Discard)
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("err = %v, want ErrChecksum", err)
	}
	after, _ := filepath.Glob(filepath.Join(os.TempDir(), "build-stream8-kernel-input-*"))
	if len(after) != len(before) {
		t.Error("staging directory left behind after failure")
	}
}

func TestStageURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pkgs/kernel-2.src.rpm" {
			http.NotFound(w, r)
			return
		}
		w.Write(payload)
	}))
	defer srv.Close()

	s, err := Stage(context.Background(), srv.URL+"/pkgs/kernel-2.src.rpm", sum(payload), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Cleanup()
	if filepath.Base(s.Path) != "kernel-2.src.rpm" {
		t.Errorf("staged name = %s", filepath.Base(s.Path))
	}
	got, _ := os.ReadFile(s.Path)
	if string(got) != string(payload) {
		t.Error("downloaded content differs")
	}

	_, err = Stage(context.Background(), srv.URL+"/missing.src.rpm", "", io.Discard)
	if !errors.Is(err, ErrDownload) {
		t.Errorf("404: err = %v, want ErrDownload", err)
	}
}
