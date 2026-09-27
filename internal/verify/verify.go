// Package verify checks the artifacts exported by the build container on the
// host side, independently of what the container reported.
package verify

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Required are the packages without which the build is not usable.
var Required = []string{"kernel", "kernel-core", "kernel-modules"}

// ErrVerify: exported artifacts are missing or inconsistent.
var ErrVerify = errors.New("artifact verification failed")

// Result lists the verified RPMs (relative to the output folder) and the
// parsed manifest.txt.
type Result struct {
	RPMs     []string
	Manifest map[string]string
}

// Check verifies out/SHA256SUMS against the RPMs under out/RPMS: every RPM
// must be listed, every listed file must exist and match, and the Required
// packages must be present.
func Check(out string) (*Result, error) {
	sums, err := readSums(filepath.Join(out, "SHA256SUMS"))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrVerify, err)
	}

	found, err := filepath.Glob(filepath.Join(out, "RPMS", "*", "*.rpm"))
	if err != nil {
		return nil, err
	}
	res := &Result{}
	for _, f := range found {
		rel, _ := filepath.Rel(out, f)
		if _, ok := sums[rel]; !ok {
			return nil, fmt.Errorf("%w: %s not listed in SHA256SUMS", ErrVerify, rel)
		}
		res.RPMs = append(res.RPMs, rel)
	}
	for rel, want := range sums {
		got, err := fileSHA256(filepath.Join(out, rel))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrVerify, err)
		}
		if got != want {
			return nil, fmt.Errorf("%w: %s: sha256 %s, SHA256SUMS says %s", ErrVerify, rel, got, want)
		}
	}
	sort.Strings(res.RPMs)

	for _, name := range Required {
		if !hasPackage(res.RPMs, name) {
			return nil, fmt.Errorf("%w: required package %s not produced", ErrVerify, name)
		}
	}

	res.Manifest, _ = readManifest(filepath.Join(out, "manifest.txt"))
	return res, nil
}

// hasPackage matches "<name>-<version>..." so that kernel-modules does not
// match kernel-modules-extra.
func hasPackage(rpms []string, name string) bool {
	for _, r := range rpms {
		base := filepath.Base(r)
		rest, ok := strings.CutPrefix(base, name+"-")
		if ok && rest != "" && rest[0] >= '0' && rest[0] <= '9' {
			return true
		}
	}
	return false
}

func readSums(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sums := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		sum, file, ok := strings.Cut(sc.Text(), "  ")
		if !ok || len(sum) != 64 {
			return nil, fmt.Errorf("malformed SHA256SUMS line: %q", sc.Text())
		}
		sums[filepath.Clean(file)] = sum
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(sums) == 0 {
		return nil, errors.New("SHA256SUMS is empty")
	}
	return sums, nil
}

func readManifest(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
