// Package runner wraps the docker CLI. The CLI is used instead of the Docker
// SDK to keep the tool dependency-free and the commands identical to the ones
// documented in the README (and swappable for podman via Bin).
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

var (
	// ErrUnavailable: docker binary missing, daemon down or no permission.
	ErrUnavailable = errors.New("docker unavailable")
	// ErrNoImage: the requested image does not exist locally.
	ErrNoImage = errors.New("image not found")
)

// Docker runs docker CLI commands. Stdout/Stderr receive streamed output of
// long-running commands (build, run).
type Docker struct {
	Bin    string
	Stdout io.Writer
	Stderr io.Writer
}

// Check verifies that the CLI exists and the daemon answers.
func (d Docker) Check(ctx context.Context) error {
	out, err := d.output(ctx, "version", "--format", "{{.Server.Version}}")
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%w: %q not found in PATH", ErrUnavailable, d.Bin)
	}
	msg := strings.TrimSpace(out)
	if strings.Contains(msg, "permission denied") {
		return fmt.Errorf("%w: permission denied on the docker socket (is your user in the docker group?): %s", ErrUnavailable, msg)
	}
	return fmt.Errorf("%w: daemon not reachable: %s", ErrUnavailable, msg)
}

// ImageID returns the ID of a local image, or ErrNoImage.
func (d Docker) ImageID(ctx context.Context, ref string) (string, error) {
	out, err := d.output(ctx, "image", "inspect", "--format", "{{.Id}}", ref)
	if err != nil {
		if strings.Contains(strings.ToLower(out), "no such image") {
			return "", ErrNoImage
		}
		return "", fmt.Errorf("docker image inspect %s: %v: %s", ref, err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

// Build runs docker build on contextDir and tags the result.
func (d Docker) Build(ctx context.Context, contextDir, tag string) error {
	return d.stream(ctx, "build", "-t", tag, contextDir)
}

// RunSpec describes one build container.
type RunSpec struct {
	Name     string
	Image    string
	InputDir string // mounted read-only at /input
	OutDir   string // mounted at /output
	Env      map[string]string
}

// Run starts the container in the foreground and returns its exit code.
// A non-nil error means the container could not be run at all.
//
// If ctx is cancelled (Ctrl+C), killing the docker client does not stop the
// container, so it is force-removed by name.
func (d Docker) Run(ctx context.Context, s RunSpec) (int, error) {
	args := []string{"run", "--rm", "--name", s.Name,
		"-v", s.InputDir + ":/input:ro,Z",
		"-v", s.OutDir + ":/output:Z",
	}
	for k, v := range s.Env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, s.Image)

	err := d.stream(ctx, args...)
	if ctx.Err() != nil {
		d.Remove(s.Name)
		return -1, ctx.Err()
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}

// Remove force-removes a container; errors are ignored (it may be gone).
func (d Docker) Remove(name string) {
	exec.Command(d.Bin, "rm", "-f", name).Run()
}

func (d Docker) output(ctx context.Context, args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, d.Bin, args...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func (d Docker) stream(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, d.Bin, args...)
	cmd.Stdout = d.Stdout
	cmd.Stderr = d.Stderr
	return cmd.Run()
}
