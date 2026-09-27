// Package buildenv embeds the Docker build context (Dockerfile, build.sh,
// vault.repo) so the build tool is self-contained: it can (re)create the
// builder image without the repository checked out next to it.
package buildenv

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed Dockerfile build.sh vault.repo
var files embed.FS

// names is the fixed file order used for hashing and extraction.
var names = []string{"Dockerfile", "build.sh", "vault.repo"}

// ImageRepo is the repository part of the builder image reference.
const ImageRepo = "stream8-kernel-builder"

// Hash returns the SHA-256 of the embedded context. Any change to the
// Dockerfile or scripts yields a new hash, and therefore a new image tag.
func Hash() string {
	h := sha256.New()
	for _, n := range names {
		b, err := files.ReadFile(n)
		if err != nil {
			panic(err) // embedded at compile time, cannot be missing
		}
		fmt.Fprintf(h, "%s\x00%d\x00", n, len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ImageRef returns the content-addressed image reference, e.g.
// stream8-kernel-builder:1a2b3c4d5e6f.
func ImageRef() string {
	return ImageRepo + ":" + Hash()[:12]
}

// Extract writes the build context into dir.
func Extract(dir string) error {
	for _, n := range names {
		b, err := files.ReadFile(n)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if n == "build.sh" {
			mode = 0o755
		}
		if err := os.WriteFile(filepath.Join(dir, n), b, mode); err != nil {
			return err
		}
	}
	return nil
}
