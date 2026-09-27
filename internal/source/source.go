// Package source stages the input SRPM (local file or URL) into a private
// directory that is mounted as /input in the build container.
package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var (
	// ErrInput: the local file is missing or not a regular file.
	ErrInput = errors.New("invalid input")
	// ErrDownload: the URL could not be fetched.
	ErrDownload = errors.New("download failed")
	// ErrChecksum: the SHA-256 does not match the expected value.
	ErrChecksum = errors.New("sha256 mismatch")
)

// Staged is an SRPM copied into its own directory.
type Staged struct {
	Dir    string // directory to mount as /input (contains only the SRPM)
	Path   string // full path of the staged SRPM
	SHA256 string
}

// Cleanup removes the staging directory.
func (s *Staged) Cleanup() error { return os.RemoveAll(s.Dir) }

// IsURL reports whether src is an http(s) URL rather than a file path.
func IsURL(src string) bool {
	u, err := url.Parse(src)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// Stage copies or downloads src into a new temporary directory and checks
// wantSHA256 when it is not empty. Progress messages go to log.
//
// Local files are always copied, never hard-linked: the staging directory is
// mounted with SELinux relabeling (:Z), which on a hard link would relabel
// the user's original file too.
func Stage(ctx context.Context, src, wantSHA256 string, log io.Writer) (*Staged, error) {
	dir, err := os.MkdirTemp("", "build-stream8-kernel-input-")
	if err != nil {
		return nil, err
	}
	s := &Staged{Dir: dir}

	var name string
	if IsURL(src) {
		u, _ := url.Parse(src)
		name = path.Base(u.Path)
	} else {
		name = filepath.Base(src)
	}
	// build.sh looks for /input/*.src.rpm; it validates the content itself.
	if !strings.HasSuffix(name, ".src.rpm") {
		name = "input.src.rpm"
	}
	s.Path = filepath.Join(dir, name)

	if IsURL(src) {
		s.SHA256, err = download(ctx, src, s.Path, log)
	} else {
		s.SHA256, err = copyFile(src, s.Path)
	}
	if err == nil && wantSHA256 != "" && !strings.EqualFold(wantSHA256, s.SHA256) {
		err = fmt.Errorf("%w: expected %s, got %s", ErrChecksum, strings.ToLower(wantSHA256), s.SHA256)
	}
	if err != nil {
		s.Cleanup()
		return nil, err
	}
	return s, nil
}

func copyFile(src, dst string) (string, error) {
	fi, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInput, err)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s is not a regular file", ErrInput, src)
	}
	in, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInput, err)
	}
	defer in.Close()
	return writeHashed(dst, in)
}

func download(ctx context.Context, rawURL, dst string, log io.Writer) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDownload, err)
	}
	client := &http.Client{Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: time.Minute,
	}}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDownload, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: %s: HTTP %s", ErrDownload, rawURL, resp.Status)
	}

	fmt.Fprintf(log, "downloading %s\n", rawURL)
	body := &progressReader{r: resp.Body, total: resp.ContentLength, log: log}
	sum, err := writeHashed(dst, body)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDownload, err)
	}
	body.done()
	return sum, nil
}

// writeHashed writes r to dst and returns the SHA-256 of what was written.
func writeHashed(dst string, r io.Reader) (string, error) {
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), r); err != nil {
		out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// progressReader prints download progress at most every 10%.
type progressReader struct {
	r     io.Reader
	total int64
	n     int64
	last  int64
	log   io.Writer
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.n += int64(n)
	if p.total > 0 {
		if pct := p.n * 100 / p.total; pct >= p.last+10 {
			p.last = pct - pct%10
			fmt.Fprintf(p.log, "  %3d%% (%d / %d MiB)\n", p.last, p.n>>20, p.total>>20)
		}
	}
	return n, err
}

func (p *progressReader) done() {
	fmt.Fprintf(p.log, "  downloaded %d MiB\n", p.n>>20)
}
