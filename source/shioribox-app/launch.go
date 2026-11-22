package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type runInfo struct {
	Start  time.Time
	Kill   func() error
	Active time.Duration // time the game's window was the foreground window
	Paused bool          // true while something else is on top
}

// RunView is what the web page sees of a running game.
type RunView struct {
	Start  string  `json:"start"`
	Active float64 `json:"active"` // seconds of real play so far
	Paused bool    `json:"paused"`
}

// fgCheck reports whether the game's own window is the one on top. Replaced in tests.
var fgCheck = foregroundIsGame

type Runner struct {
	mu sync.Mutex
	m  map[string]*runInfo
}

func newRunner() *Runner { return &Runner{m: map[string]*runInfo{}} }

func (r *Runner) Snapshot() map[string]RunView {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]RunView{}
	for id, ri := range r.m {
		out[id] = RunView{Start: ri.Start.Format(time.RFC3339), Active: ri.Active.Seconds(), Paused: ri.Paused}
	}
	return out
}

// tickFocus adds the time since the last tick to the session if the game is on top. Returns the new tick time.
func (r *Runner) tickFocus(ri *runInfo, last time.Time, onTop bool) time.Time {
	now := time.Now()
	dt := now.Sub(last)
	if dt > 2*time.Second {
		dt = 2 * time.Second // the PC slept or the app stalled: don't count the gap
	}
	r.mu.Lock()
	if onTop {
		ri.Active += dt
	}
	ri.Paused = !onTop
	r.mu.Unlock()
	return now
}

func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	inQ, has := false, false
	for _, c := range s {
		switch {
		case c == '"':
			inQ = !inQ
			has = true
		case (c == ' ' || c == '\t') && !inQ:
			if has || cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteRune(c)
		}
	}
	if has || cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

const (
	adoptWindow   = 20 * time.Second // how long to look for the game after a launcher exits
	adoptMaxAfter = 30 * time.Second // a launcher that stayed this long was itself holding the game
)

// viaLauncher runs a launcher program (a locale emulator or custom launcher) that starts the game
// and usually exits straight away. Once it has exited we find the game's own process and time that.
func viaLauncher(cmd *exec.Cmd, exe string, start time.Time) (wait func() time.Time, kill func() error) {
	var mu sync.Mutex
	var pid uint32
	wait = func() time.Time {
		cmd.Wait()
		launcherEnd := time.Now()
		if launcherEnd.Sub(start) > adoptMaxAfter {
			return launcherEnd
		}
		var found uint32
		for deadline := time.Now().Add(adoptWindow); time.Now().Before(deadline); time.Sleep(400 * time.Millisecond) {
			if found = findGameProcess(exe, start); found != 0 {
				break
			}
		}
		if found == 0 {
			logf("launcher exited but the game process was not found: %s", exe)
			return launcherEnd
		}
		mu.Lock()
		pid = found
		mu.Unlock()
		waitPID(found)
		return time.Now()
	}
	kill = func() error {
		mu.Lock()
		p := pid
		mu.Unlock()
		if p != 0 {
			return killPID(p)
		}
		return cmd.Process.Kill()
	}
	return
}

func (a *App) launch(id string) error {
	var g Game
	var st Settings
	found := false
	a.store.View(func(l *Library) {
		if x := findGame(l, id); x != nil {
			g, st, found = *x, l.Settings, true
		}
	})
	if !found {
		return errors.New("game not found")
	}
	if g.ExePath == "" {
		return errors.New("no executable set for this game — edit it and choose one")
	}
	if _, err := os.Stat(g.ExePath); err != nil {
		return fmt.Errorf("executable not found: %s", g.ExePath)
	}
	a.run.mu.Lock()
	if _, ok := a.run.m[id]; ok {
		a.run.mu.Unlock()
		return errors.New("already running")
	}
	a.run.mu.Unlock()

	extra := splitArgs(g.LaunchArgs)
	var cmd *exec.Cmd
	launcher := false
	switch {
	case g.Locale == "ja" && st.LocaleEngine == "classic":
		es := engineStatus(a.dataDir)
		if !es.Ready {
			return errors.New(es.Problem)
		}
		c, err := engineCommand(es.Dir, g.ExePath, extra)
		if err != nil {
			return err
		}
		c.Dir = filepath.Dir(g.ExePath)
		hideWindow(c)
		start := time.Now()
		out, err := c.Output()
		pid, perr := parseLauncherOutput(string(out))
		if perr != nil {
			if err != nil || strings.TrimSpace(string(out)) != "" {
				msg := perr.Error()
				if strings.Contains(msg, "elevation") {
					msg += " — start ShioriBox as administrator to launch it"
				}
				return fmt.Errorf("Japanese locale: %s", msg)
			}
			return fmt.Errorf("Japanese locale: %v", err)
		}
		a.trackRun(id, g, st, start, func() time.Time { waitPID(pid); return time.Now() }, func() error { return killPID(pid) })
		return nil
	case g.Locale == "ja" && st.LocaleEngine == "le":
		if err := checkLEGame(g.ExePath); err != nil {
			return err
		}
		ls := leStatus(st)
		if !ls.Available {
			return fmt.Errorf("Locale Emulator was not found: %s Open Settings → Japanese locale for setup steps", ls.Problem)
		}
		prog, args := leCommand(ls, g.ExePath, extra)
		cmd = exec.Command(prog, args...)
		hideWindow(cmd)
		launcher = true
	case g.Locale == "ja" && st.LocaleEngine == "custom":
		if st.LocalePath == "" {
			return errors.New("no locale emulator program is set — choose one in Settings → Japanese locale")
		}
		if _, err := os.Stat(st.LocalePath); err != nil {
			return fmt.Errorf("locale emulator not found: %s", st.LocalePath)
		}
		cmd = exec.Command(st.LocalePath, customLocaleArgs(st.LocaleArgs, g.ExePath, extra)...)
		hideWindow(cmd)
		launcher = true
	case g.Locale == "ja":
		es := engineStatus(a.dataDir)
		if !es.Ready {
			return errors.New(es.Problem)
		}
		args := []string{g.ExePath}
		for _, x := range extra {
			args = append(args, quoteArg(x))
		}
		cmd = exec.Command(filepath.Join(es.Dir, "jle.exe"), args...)
		hideWindow(cmd)
		launcher = true
	case g.UseWrapper && st.WrapperPath != "":
		var args []string
		for _, t := range splitArgs(st.WrapperArgs) {
			args = append(args, strings.ReplaceAll(t, "{exe}", g.ExePath))
		}
		cmd = exec.Command(st.WrapperPath, append(args, extra...)...)
		launcher = true
	default:
		cmd = exec.Command(g.ExePath, extra...)
	}
	cmd.Dir = filepath.Dir(g.ExePath)
	start := time.Now()
	if err := cmd.Start(); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "elevation") {
			msg += " — this game needs administrator rights; start ShioriBox as administrator to launch it"
		}
		return fmt.Errorf("could not start: %s", msg)
	}
	var wait func() time.Time
	var kill func() error
	if launcher {
		wait, kill = viaLauncher(cmd, g.ExePath, start)
	} else {
		wait = func() time.Time { cmd.Wait(); return time.Now() }
		kill = func() error { return cmd.Process.Kill() }
	}
	a.trackRun(id, g, st, start, wait, kill)
	return nil
}

// trackRun records a running game, times it and handles the end-of-session bookkeeping.
func (a *App) trackRun(id string, g Game, st Settings, start time.Time, wait func() time.Time, kill func() error) {
	ri := &runInfo{Start: start, Kill: kill, Paused: true}
	a.run.mu.Lock()
	a.run.m[id] = ri
	a.run.mu.Unlock()
	done := make(chan struct{})
	// Only time with the game's window on top counts as playtime.
	go func() {
		last := time.Now()
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				last = a.run.tickFocus(ri, last, fgCheck(g.ExePath))
			}
		}
	}()
	a.store.Update(func(l *Library) error {
		if x := findGame(l, id); x != nil {
			x.LastPlayed = start
			if x.Status == "planned" {
				x.Status = "playing"
			}
		}
		return nil
	})

	go func() {
		wait()
		close(done)
		a.run.mu.Lock()
		secs := int(ri.Active.Seconds())
		delete(a.run.m, id)
		a.run.mu.Unlock()
		if secs >= st.MinSessionSec {
			a.store.Update(func(l *Library) error {
				if x := findGame(l, id); x != nil {
					x.Sessions = append(x.Sessions, Session{Start: start, Seconds: secs})
				}
				return nil
			})
		}
		if st.AutoBackup && g.SavePath != "" {
			if _, err := a.backupGame(id); err != nil {
				logf("auto-backup for %s failed: %v", g.Title, err)
			}
		}
	}()
}

func (a *App) stop(id string) error {
	a.run.mu.Lock()
	ri, ok := a.run.m[id]
	a.run.mu.Unlock()
	if !ok {
		return errors.New("not running")
	}
	return ri.Kill()
}
