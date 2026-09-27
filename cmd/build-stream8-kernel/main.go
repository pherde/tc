// Command build-stream8-kernel builds a CentOS Stream 8 kernel SRPM in the
// Docker build environment and exports the binary RPMs.
//
//	build-stream8-kernel [flags] <src.rpm|url> <outfolder>
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	buildenv "stream8-kernel/docker"
	"stream8-kernel/internal/clean"
	"stream8-kernel/internal/runner"
	"stream8-kernel/internal/source"
	"stream8-kernel/internal/verify"
)

// Exit codes. 2-5 are the build.sh codes, passed through unchanged.
const (
	exitOK          = 0
	exitUsage       = 1
	exitInput       = 2
	exitBuildDeps   = 3
	exitRPMBuild    = 4
	exitExport      = 5
	exitDocker      = 6
	exitFetch       = 7
	exitImageBuild  = 8
	exitVerify      = 9
	exitClean       = 10
	exitInterrupted = 130
)

// codedError carries the process exit code with the message.
type codedError struct {
	code int
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

func fail(code int, format string, a ...any) error {
	return &codedError{code, fmt.Errorf(format, a...)}
}

type options struct {
	src, out     string
	sha256       string
	jobs         int
	rpmbuildOpts string
	image        string
	rebuildImage bool
	force        bool
	quiet        bool
	dockerBin    string

	clean     bool
	dryRun    bool
	cleanDirs []string
}

// defaultCleanDir is cleaned when --clean is given without a folder.
const defaultCleanDir = "out"

const usage = `usage: build-stream8-kernel [flags] <src.rpm|url> <outfolder>
       build-stream8-kernel --clean [--dry-run] [folder...]

Builds a CentOS Stream 8 kernel SRPM inside Docker and exports the binary
RPMs, build.log, manifest.txt, toolchain.txt and SHA256SUMS to <outfolder>.

With --clean, removes those build results from each folder (default: ./out),
including result folders nested below it. Other files are kept.

flags:
`

func main() {
	opts, err := parseArgs(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(exitOK)
	}
	if err != nil {
		os.Exit(exitUsage)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runFn := run
	if opts.clean {
		runFn = func(_ context.Context, o *options) error { return runClean(o, os.Stdout) }
	}
	if err := runFn(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "build-stream8-kernel: error: %v\n", err)
		os.Exit(exitCode(err))
	}
}

func parseArgs(args []string, stderr io.Writer) (*options, error) {
	o := &options{}
	fs := flag.NewFlagSet("build-stream8-kernel", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	fs.StringVar(&o.sha256, "sha256", "", "expected SHA-256 of the SRPM (recommended for URLs)")
	fs.IntVar(&o.jobs, "jobs", 0, "make parallelism (default: nproc inside the container)")
	fs.StringVar(&o.rpmbuildOpts, "rpmbuild-opts", "", `rpmbuild options (default: "--with baseonly --without debuginfo --without kabidwchk")`)
	fs.StringVar(&o.image, "image", "", "builder image to use (default: built from the embedded context, tagged "+buildenv.ImageRef()+")")
	fs.BoolVar(&o.rebuildImage, "rebuild-image", false, "rebuild the builder image even if it exists")
	fs.BoolVar(&o.force, "force", false, "allow writing into an outfolder that already contains build results")
	fs.BoolVar(&o.quiet, "quiet", false, "do not stream the build output (it is still saved to build.log)")
	fs.StringVar(&o.dockerBin, "docker", "docker", "docker CLI binary")
	fs.BoolVar(&o.clean, "clean", false, "remove build results from the given folders (default: ./out) instead of building")
	fs.BoolVar(&o.dryRun, "dry-run", false, "with --clean: only list what would be removed")

	// Accept flags before, between and after the positional arguments.
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
	if o.clean {
		o.cleanDirs = pos
		if len(o.cleanDirs) == 0 {
			o.cleanDirs = []string{defaultCleanDir}
		}
		return o, nil
	}
	if o.dryRun {
		fmt.Fprintln(stderr, "--dry-run is only valid with --clean")
		return nil, errors.New("--dry-run without --clean")
	}
	if len(pos) != 2 {
		fs.Usage()
		return nil, errors.New("expected <src.rpm|url> <outfolder>")
	}
	if o.jobs < 0 {
		fmt.Fprintln(stderr, "--jobs must be positive")
		return nil, errors.New("bad --jobs")
	}
	o.src, o.out = pos[0], pos[1]
	return o, nil
}

func run(ctx context.Context, o *options) error {
	start := time.Now()
	logf := func(format string, a ...any) { fmt.Fprintf(os.Stderr, "==> "+format+"\n", a...) }

	out, err := prepareOutDir(o.out, o.force)
	if err != nil {
		return err
	}

	d := runner.Docker{Bin: o.dockerBin, Stdout: os.Stdout, Stderr: os.Stderr}
	if o.quiet {
		d.Stdout, d.Stderr = io.Discard, io.Discard
	}
	if err := d.Check(ctx); err != nil {
		return fail(exitDocker, "%v", err)
	}

	logf("staging SRPM %s", o.src)
	staged, err := source.Stage(ctx, o.src, o.sha256, os.Stderr)
	switch {
	case ctx.Err() != nil:
		return fail(exitInterrupted, "interrupted")
	case errors.Is(err, source.ErrInput):
		return fail(exitInput, "%v", err)
	case err != nil:
		return fail(exitFetch, "%v", err)
	}
	defer staged.Cleanup()
	logf("SRPM sha256 %s", staged.SHA256)

	image, imageID, err := ensureImage(ctx, d, o, logf)
	if err != nil {
		return err
	}

	env := map[string]string{
		"HOST_UID":  strconv.Itoa(os.Getuid()),
		"HOST_GID":  strconv.Itoa(os.Getgid()),
		"IMAGE_REF": image + "@" + imageID,
	}
	if o.jobs > 0 {
		env["JOBS"] = strconv.Itoa(o.jobs)
	}
	if o.rpmbuildOpts != "" {
		env["RPMBUILD_OPTS"] = o.rpmbuildOpts
	}
	name := fmt.Sprintf("build-stream8-kernel-%d-%d", os.Getpid(), time.Now().Unix())

	logf("building in container %s (image %s); log: %s", name, image, filepath.Join(out, "build.log"))
	code, err := d.Run(ctx, runner.RunSpec{
		Name: name, Image: image, InputDir: staged.Dir, OutDir: out, Env: env,
	})
	if ctx.Err() != nil {
		return fail(exitInterrupted, "interrupted; container %s removed", name)
	}
	if err != nil {
		return fail(exitDocker, "docker run: %v", err)
	}
	if code != 0 {
		return containerError(code, out)
	}

	res, err := verify.Check(out)
	if err != nil {
		return fail(exitVerify, "%v", err)
	}
	printSummary(os.Stdout, out, res, time.Since(start))
	return nil
}

// runClean removes build results from each folder in o.cleanDirs.
func runClean(o *options, w io.Writer) error {
	verb := "removed"
	if o.dryRun {
		verb = "would remove"
	}
	var failed []string
	for _, dir := range o.cleanDirs {
		rep, err := clean.Dir(dir, o.dryRun)
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(w, "%s: does not exist, nothing to clean\n", dir)
			continue
		}
		if rep != nil {
			for _, p := range rep.Removed {
				fmt.Fprintf(w, "%s %s\n", verb, p)
			}
			for _, p := range rep.Kept {
				fmt.Fprintf(w, "kept %s (not a build artifact)\n", p)
			}
			fmt.Fprintf(w, "%s: %s %d artifacts (%.1f MiB), %d empty folders\n",
				dir, verb, len(rep.Removed), float64(rep.Bytes)/(1<<20), len(rep.Dirs))
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "build-stream8-kernel: error: %s: %v\n", dir, err)
			failed = append(failed, dir)
		}
	}
	if len(failed) > 0 {
		return fail(exitClean, "could not clean %s", strings.Join(failed, ", "))
	}
	return nil
}

// prepareOutDir creates the output folder and refuses to overwrite previous
// results unless force is set.
func prepareOutDir(dir string, force bool) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fail(exitUsage, "%v", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fail(exitUsage, "cannot create outfolder: %v", err)
	}
	if !force {
		for _, n := range []string{"RPMS", "build.log", "manifest.txt", "SHA256SUMS"} {
			if _, err := os.Stat(filepath.Join(abs, n)); err == nil {
				return "", fail(exitUsage, "%s already contains build results (%s); use --force to overwrite", abs, n)
			}
		}
	}
	return abs, nil
}

// ensureImage returns the image reference and ID to run, building the
// embedded context when the content-addressed image does not exist yet.
func ensureImage(ctx context.Context, d runner.Docker, o *options, logf func(string, ...any)) (string, string, error) {
	if o.image != "" {
		id, err := d.ImageID(ctx, o.image)
		if err != nil {
			return "", "", fail(exitDocker, "image %s: %v", o.image, err)
		}
		return o.image, id, nil
	}

	ref := buildenv.ImageRef()
	id, err := d.ImageID(ctx, ref)
	if err == nil && !o.rebuildImage {
		logf("using image %s", ref)
		return ref, id, nil
	}
	if err != nil && !errors.Is(err, runner.ErrNoImage) {
		return "", "", fail(exitDocker, "%v", err)
	}

	dir, err := os.MkdirTemp("", "build-stream8-kernel-context-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(dir)
	if err := buildenv.Extract(dir); err != nil {
		return "", "", err
	}
	logf("building image %s from the embedded context", ref)
	if err := d.Build(ctx, dir, ref); err != nil {
		if ctx.Err() != nil {
			return "", "", fail(exitInterrupted, "interrupted")
		}
		return "", "", fail(exitImageBuild, "docker build of %s failed: %v", ref, err)
	}
	id, err = d.ImageID(ctx, ref)
	if err != nil {
		return "", "", fail(exitImageBuild, "image %s missing after build: %v", ref, err)
	}
	return ref, id, nil
}

// containerError translates a build.sh / docker run exit code.
func containerError(code int, out string) error {
	logPath := filepath.Join(out, "build.log")
	switch code {
	case exitInput:
		return fail(code, "the input is not a valid source RPM (see %s)", logPath)
	case exitBuildDeps:
		return fail(code, "installing build dependencies failed; is vault.centos.org reachable? (see %s)", logPath)
	case exitRPMBuild:
		return fail(code, "rpmbuild failed; last lines of %s:\n%s", logPath, tail(logPath, 20))
	case exitExport:
		return fail(code, "exporting artifacts to %s failed (see %s)", out, logPath)
	case 125, 126, 127:
		return fail(exitDocker, "docker could not start the container (exit %d)", code)
	default:
		return fail(exitRPMBuild, "build container exited with code %d (see %s)", code, logPath)
	}
}

func exitCode(err error) int {
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.code
	}
	return exitUsage
}

// tail returns the last n lines of a file, or a placeholder if unreadable.
func tail(path string, n int) string {
	f, err := os.Open(path)
	if err != nil {
		return "  (build.log not readable)"
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return "  " + strings.Join(lines, "\n  ")
}

func printSummary(w io.Writer, out string, res *verify.Result, elapsed time.Duration) {
	fmt.Fprintf(w, "\nBuild succeeded: %s\n", res.Manifest["nvr"])
	fmt.Fprintf(w, "Output: %s\n", out)
	for _, r := range res.RPMs {
		size := int64(0)
		if fi, err := os.Stat(filepath.Join(out, r)); err == nil {
			size = fi.Size()
		}
		fmt.Fprintf(w, "  %-60s %6.1f MiB\n", r, float64(size)/(1<<20))
	}
	fmt.Fprintf(w, "SHA256SUMS verified for %d RPMs. rpmbuild: %ss, total: %s\n",
		len(res.RPMs), res.Manifest["build_seconds"], elapsed.Round(time.Second))
}
