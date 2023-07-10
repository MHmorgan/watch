package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"hash"
	"hash/adler32"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

var (
	help    = flag.Bool("h", false, "show help")
	verbose = flag.Bool("v", false, "verbose output")
	delay   = flag.Int("d", 1, "delay in seconds between commands")
	timeout = flag.Int("t", 60, "WatchCommand timeout in seconds")
	pathStr = flag.String("p", "", "paths to watch for changes (optional)")
	scrType = flag.String("s", "plain", "screen type (plain, vt100)")
)

const usage = `               _       _     
__      ____ _| |_ ___| |__  
\ \ /\ / / _' | __/ __| '_ \ 
 \ V  V / (_| | || (__| | | |
  \_/\_/ \__,_|\__\___|_| |_|

Usage: watch [options] command [args...]

Watch a command and its output. There is a delay between commands (-d)
and if a timeout (-t) is reached then watch will exit.

The paths (-p) are a space separated list of paths to watch for changes.
Directories are searched recursively. When no changes are detected the
command is not run.

The screen type determines how the output is displayed. The default, plain,
will just print the output to stdout with no formatting.

Screen types
    plain
        Plain text output.
    vt100
        VT100 terminal output.

Options:
`

func main() {
	flag.Parse()

	if *help {
		fmt.Print(usage)
		flag.PrintDefaults()
		os.Exit(0)
	}

	if flag.NArg() < 1 {
		bail("No command specified.")
	}

	cmd := WatchCommand{
		name:    flag.Arg(0),
		args:    flag.Args()[1:],
		timeout: time.Duration(*timeout) * time.Second,
	}
	delay := time.Duration(*delay) * time.Second
	paths := NewWatchPaths(*pathStr)

	screen, ok := screens[*scrType]
	if !ok {
		fmt.Printf("Unknown screen type: %v\n", *scrType)
		os.Exit(2)
	}
	screen.Name(cmd.name)
	screen.Setup()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, os.Kill)
	go func() {
		<-stop
		screen.Teardown()
		os.Exit(0)
	}()

	debug("watching %q", flag.Args())
	debug("delay %v", delay)
	debug("timeout %v", cmd.timeout)

	// WATCH

	first := true
	for {
		time.Sleep(delay)
		paths.update()
		if !paths.hasChanged() {
			continue
		}

		cmd.run()

		if err, ok := cmd.err.(*exec.ExitError); ok {
			screen.Status("exit code %v", err.ExitCode())
		} else if errors.Is(cmd.err, context.DeadlineExceeded) {
			bail("timeout after %v", cmd.timeout)
		} else if cmd.err != nil {
			bail("executing %q with args %q: %v", cmd.name, cmd.args, cmd.err)
		} else if cmd.output() == "" {
			screen.Status("no output")
		} else {
			screen.Status("")
		}

		if cmd.hasChanged() || first {
			fmt.Fprint(screen, cmd.buf.String())
		}
		first = false
	}
}

// -----------------------------------------------------------------------------
// COMMAND

type WatchCommand struct {
	name    string
	args    []string
	timeout time.Duration

	buf  bytes.Buffer
	prev uint32
	err  error
}

func (c *WatchCommand) hasChanged() bool {
	return c.prev != adler32.Checksum(c.buf.Bytes())
}

func (c *WatchCommand) run() {
	c.prev = adler32.Checksum(c.buf.Bytes())
	c.buf.Reset()

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	cmd := exec.CommandContext(ctx, c.name, c.args...)
	cmd.Stdout = &c.buf
	cmd.Stderr = &c.buf

	c.err = cmd.Run()
	if ctx.Err() != nil {
		c.err = ctx.Err()
	}
	cancel() // avoid leaking context
}

func (c *WatchCommand) output() string {
	return strings.TrimSpace(c.buf.String())
}

// -----------------------------------------------------------------------------
// PATHS

type WatchPaths struct {
	files []string
	dirs  []string
	prev  uint32
	hash  hash.Hash32
}

func NewWatchPaths(pathStr string) *WatchPaths {
	if pathStr == "" {
		return nil
	}
	paths := strings.Fields(pathStr)

	wp := &WatchPaths{
		files: make([]string, 0),
		dirs:  make([]string, 0),
		hash:  adler32.New(),
	}
	for _, p := range paths {
		if stat, err := os.Stat(p); err != nil {
			bail("invalid path %q: %v", p, err)
		} else if stat.IsDir() {
			wp.dirs = append(wp.dirs, p)
		} else {
			wp.files = append(wp.files, p)
		}
	}
	return wp
}

func (p *WatchPaths) hasChanged() bool {
	if p == nil {
		return true
	}
	return p.prev != p.hash.Sum32()
}

func (p *WatchPaths) update() {
	if p == nil {
		return
	}

	p.prev = p.hash.Sum32()
	p.hash.Reset()

	files := make(chan string, 1)
	go func() {
		for _, f := range p.files {
			files <- f
		}
		for _, dir := range p.dirs {
			filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					bail("error walking %q: %v", path, err)
				} else if info.Mode().IsRegular() {
					files <- path
				}
				return nil
			})
		}
		close(files)
	}()

	for f := range files {
		if data, err := os.ReadFile(f); err != nil {
			bail("error reading %q: %v", f, err)
		} else {
			p.hash.Write(data)
		}
	}
}

// -----------------------------------------------------------------------------
// HELPERS

func bail(msg string, args ...any) {
	txt := "ERROR: " + fmt.Sprintf(msg, args...)
	if !strings.HasSuffix(txt, "\n") {
		txt += "\n"
	}
	fmt.Fprint(os.Stderr, txt)
	os.Exit(1)
}

func warn(msg string, args ...any) {
	txt := "ERROR: " + fmt.Sprintf(msg, args...)
	if !strings.HasSuffix(txt, "\n") {
		txt += "\n"
	}
	if *verbose {
		fmt.Fprint(os.Stderr, txt)
	}
}

func debug(msg string, args ...any) {
	txt := "DEBUG: " + fmt.Sprintf(msg, args...)
	if !strings.HasSuffix(txt, "\n") {
		txt += "\n"
	}
	if *verbose {
		fmt.Fprint(os.Stderr, txt)
	}
}
