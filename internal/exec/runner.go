// Package exec runs a document's shell code blocks and streams their
// output line by line. It is a pure, Bubble-Tea-free package: callers wrap
// its channels in tea.Cmd themselves.
package exec

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// Result is the outcome of a finished block run.
type Result struct {
	ExitCode int
	Err      error // non-nil on start failure or a non-exit error
}

// Run is one block's execution within a ShellSession.
//
// Started closes once the block's script has actually been written to the
// shell (as opposed to merely queued behind another still-running block).
// Lines yields stdout/stderr merged, line by line, in real time, and is
// closed when the block's output ends. Done then receives exactly one
// Result and is itself closed.
type Run struct {
	Started chan struct{}
	Lines   chan string
	Done    chan Result
}

type runRequest struct {
	script string
	out    *Run
}

// ShellSession is one persistent "sh" process backing every runnable code
// block in a single open document. Running two blocks in the same
// ShellSession behaves like two cells in the same notebook kernel: an
// `export FOO=bar` in one block is visible to `echo $FOO` in the next,
// because both execute in the same shell process rather than a fresh one
// per block.
//
// Only one block runs at a time — a real shell can't do otherwise. Run
// requests queue and are dispatched strictly in order by a single internal
// goroutine, so callers never need to wait for one run to finish before
// starting the next.
type ShellSession struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser

	id   int64 // process start time, for marker uniqueness across sessions
	seq  atomic.Int64
	runs chan *runRequest

	rawLines chan string // merged stdout+stderr, fed by a single reader goroutine

	closed atomic.Bool
	cancel context.CancelFunc
}

// StartShell launches the persistent shell rooted at dir. It never blocks.
func StartShell(dir string) (*ShellSession, error) {
	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, "sh", "-s")
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}

	// stdout and stderr share one pipe so merged output preserves real
	// execution order — reading two separate pipes concurrently can't
	// guarantee that, and this session's completion marker (written to
	// stdout) must be seen strictly after everything printed before it.
	pr, pw, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, err
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		cancel()
		pr.Close()
		pw.Close()
		return nil, err
	}
	pw.Close() // parent's copy; child holds its own, so this lets pr see EOF on exit

	s := &ShellSession{
		cmd:      cmd,
		stdin:    stdin,
		id:       nowUnixNano(),
		runs:     make(chan *runRequest, 8),
		rawLines: make(chan string),
		cancel:   cancel,
	}

	go s.readRaw(pr)

	// Make the shell itself ignore SIGINT via a caught (non-empty) trap,
	// rather than SIG_IGN — a caught signal resets to default disposition
	// across exec, so any external command the shell later runs still dies
	// normally on SIGINT, while the shell process itself survives Cancel's
	// process-group signal. Without this, Cancel kills the whole session on
	// every use, since a non-interactive shell's default SIGINT action is
	// to terminate.
	//
	// Block until the trap is confirmed installed (a marker printed right
	// after) before returning — otherwise a Cancel() racing right after
	// StartShell could fire before the trap is live and kill the session on
	// its very first use.
	initMarker := fmt.Sprintf("__runmd_init_%x__", s.id)
	if _, err := fmt.Fprintf(stdin, "trap ':' INT\nprintf '%s\\n'\n", initMarker); err != nil {
		cancel()
		_ = stdin.Close()
		pr.Close()
		return nil, err
	}
	for line := range s.rawLines {
		if line == initMarker {
			break
		}
	}

	go s.dispatchLoop()

	return s, nil
}

// Closed reports whether the shell process has ended — either explicitly
// via Close, or because it died on its own (killed by Cancel, or the
// script itself called `exit`). Callers should start a fresh ShellSession
// once this is true.
func (s *ShellSession) Closed() bool {
	return s.closed.Load()
}

// Run queues script to execute next in the shared shell and returns
// immediately with channels streaming its result — safe to call even while
// another block is still running; it simply waits its turn.
func (s *ShellSession) Run(script string) *Run {
	r := &Run{
		Started: make(chan struct{}),
		Lines:   make(chan string, 256),
		Done:    make(chan Result, 1),
	}
	s.runs <- &runRequest{script: script, out: r}
	return r
}

// Cancel interrupts whatever is currently executing in the shell (SIGINT to
// the whole process group, so it reaches an external command the shell is
// waiting on) without necessarily ending the session — env vars and cwd
// set by earlier blocks normally survive. If the interrupted command was a
// shell builtin (or the shell has nothing running), the non-interactive
// shell's default SIGINT disposition may terminate it instead; the next
// Run call transparently gets a fresh session in that case (see Closed).
func (s *ShellSession) Cancel() {
	if s.cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGINT)
}

// Close terminates the session's shell process entirely — used when the
// open document changes (new directory, so a fresh shell is warranted
// anyway) or the app is quitting.
func (s *ShellSession) Close() {
	s.closed.Store(true)
	if s.cmd.Process != nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
	}
	s.cancel()
	_ = s.stdin.Close()
}

func (s *ShellSession) readRaw(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		s.rawLines <- scanner.Text()
	}
	close(s.rawLines)
}

func (s *ShellSession) dispatchLoop() {
	for req := range s.runs {
		if s.Closed() {
			s.failRequest(req, errors.New("shell session ended"))
			continue
		}
		s.runOne(req)
	}
}

// runOne writes req's script to the shell, then blocks reading merged
// output until it sees this run's unique completion marker (appended after
// the script), forwarding every other line to req.out.Lines.
func (s *ShellSession) runOne(req *runRequest) {
	marker := fmt.Sprintf("__runmd_%x_%d__", s.id, s.seq.Add(1))

	script := req.script
	if !strings.HasSuffix(script, "\n") {
		script += "\n"
	}
	if _, err := io.WriteString(s.stdin, script); err != nil {
		s.closed.Store(true)
		s.failRequest(req, err)
		return
	}
	// $? must be captured immediately after the script, before printf
	// itself can change it — the shell-side %%d gives us the block's real
	// exit status, not printf's.
	if _, err := fmt.Fprintf(s.stdin, "printf '%s:%%d\\n' \"$?\"\n", marker); err != nil {
		s.closed.Store(true)
		s.failRequest(req, err)
		return
	}
	close(req.out.Started)

	prefix := marker + ":"
	for line := range s.rawLines {
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			code, _ := strconv.Atoi(rest)
			close(req.out.Lines)
			req.out.Done <- Result{ExitCode: code}
			close(req.out.Done)
			return
		}
		req.out.Lines <- line
	}

	// rawLines closed before the marker showed up: the shell process itself
	// ended (killed via Cancel, crashed, or the script called `exit`).
	s.closed.Store(true)
	close(req.out.Lines)
	req.out.Done <- Result{ExitCode: -1, Err: errors.New("shell session ended")}
	close(req.out.Done)
}

func nowUnixNano() int64 {
	return time.Now().UnixNano()
}

func (s *ShellSession) failRequest(req *runRequest, err error) {
	close(req.out.Started)
	close(req.out.Lines)
	req.out.Done <- Result{ExitCode: -1, Err: err}
	close(req.out.Done)
}
