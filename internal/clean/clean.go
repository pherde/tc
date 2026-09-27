// Package clean removes build results written by the build tool. Only the
// artifacts the tool itself produces are deleted; anything else found in an
// output folder is kept and reported, so a mistyped target can't wipe
// unrelated data.
package clean

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Artifacts are the entries build.sh writes into an output folder.
var Artifacts = []string{"RPMS", "build.log", "manifest.txt", "toolchain.txt", "SHA256SUMS"}

// Report describes what a clean did (or would do, in dry-run mode).
type Report struct {
	Removed []string // artifacts removed
	Kept    []string // files not produced by the tool, left in place
	Dirs    []string // directories removed because they ended up empty
	Bytes   int64    // size of the removed artifacts
}

// Dir cleans every output folder found under target (target included). A
// folder is recognized by containing at least one of the Artifacts.
// Directories left empty are removed, target included. With dryRun nothing
// is deleted.
func Dir(target string, dryRun bool) (*Report, error) {
	fi, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", target)
	}

	rep := &Report{}
	var dirs []string
	err = filepath.WalkDir(target, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		dirs = append(dirs, p)
		if filepath.Base(p) == "RPMS" && isResultDir(filepath.Dir(p)) {
			return filepath.SkipDir // removed as a whole by its parent
		}
		if !isResultDir(p) {
			return nil
		}
		for _, a := range Artifacts {
			ap := filepath.Join(p, a)
			if _, err := os.Lstat(ap); err != nil {
				continue
			}
			rep.Bytes += size(ap)
			rep.Removed = append(rep.Removed, ap)
			if !dryRun {
				if err := os.RemoveAll(ap); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return rep, err
	}

	// Deepest first, so parents become empty after their children go.
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	removed := map[string]bool{}
	for _, d := range dirs {
		if isArtifact(d, rep.Removed) {
			continue
		}
		left, err := remaining(d, rep.Removed, removed)
		if err != nil {
			return rep, err
		}
		if len(left) > 0 {
			for _, f := range left {
				if fi, err := os.Lstat(f); err == nil && !fi.IsDir() {
					rep.Kept = append(rep.Kept, f)
				}
			}
			continue
		}
		removed[d] = true
		rep.Dirs = append(rep.Dirs, d)
		if !dryRun {
			if err := os.Remove(d); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return rep, err
			}
		}
	}
	sort.Strings(rep.Kept)
	return rep, nil
}

func isResultDir(dir string) bool {
	for _, a := range Artifacts {
		if _, err := os.Lstat(filepath.Join(dir, a)); err == nil {
			return true
		}
	}
	return false
}

func isArtifact(p string, removed []string) bool {
	for _, r := range removed {
		if p == r {
			return true
		}
	}
	return false
}

// remaining lists the entries of dir that are neither removed artifacts nor
// removed (empty) subdirectories. It works in dry-run mode, where nothing was
// actually deleted.
func remaining(dir string, artifacts []string, removedDirs map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var left []string
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if isArtifact(p, artifacts) || removedDirs[p] {
			continue
		}
		left = append(left, p)
	}
	return left, nil
}

func size(p string) int64 {
	var n int64
	filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}
