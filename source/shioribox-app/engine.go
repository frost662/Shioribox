package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ShioriLocale is the built-in locale engine (separate project in ../shiorilocale). The four
// binaries are embedded and unpacked next to the data folder the first time they are needed.
//
//go:embed engine/*
var engineFS embed.FS

type EngineStatus struct {
	Ready   bool   `json:"ready"`
	Dir     string `json:"dir"`
	Problem string `json:"problem,omitempty"`
}

func engineDir(dataDir string) string { return filepath.Join(dataDir, "engine") }

// prepareEngine unpacks the engine files, replacing any that differ from the embedded copy.
func prepareEngine(dataDir string) (string, error) {
	dir := engineDir(dataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ents, err := engineFS.ReadDir("engine")
	if err != nil {
		return "", err
	}
	for _, e := range ents {
		want, err := engineFS.ReadFile("engine/" + e.Name())
		if err != nil {
			return "", err
		}
		p := filepath.Join(dir, e.Name())
		have, rerr := os.ReadFile(p)
		if rerr == nil && (bytes.Equal(have, want) || strings.HasSuffix(e.Name(), ".ini")) {
			continue // up to date, or a settings file the user may have edited
		}
		if err := os.WriteFile(p, want, 0o755); err != nil {
			return "", fmt.Errorf("could not unpack the locale engine (%v) — is your antivirus blocking %s?", err, dir)
		}
	}
	return dir, nil
}

func engineStatus(dataDir string) EngineStatus {
	dir, err := prepareEngine(dataDir)
	if err != nil {
		return EngineStatus{Problem: err.Error()}
	}
	return EngineStatus{Ready: true, Dir: dir}
}

// engineCommand builds the launcher command for exe, picking the 32- or 64-bit build.
func engineCommand(dir, exe string, gameArgs []string) (*exec.Cmd, error) {
	bits := exeBits(exe)
	if bits == 0 {
		return nil, errors.New("could not read this executable — is it a Windows program?")
	}
	args := []string{
		"--dll", filepath.Join(dir, "sbl"+strconv.Itoa(bits)+".dll"),
		"--exe", exe,
		"--dir", filepath.Dir(exe),
	}
	if len(gameArgs) > 0 {
		q := make([]string, len(gameArgs))
		for i, a := range gameArgs {
			q[i] = quoteArg(a)
		}
		args = append(args, "--args", strings.Join(q, " "))
	}
	return exec.Command(filepath.Join(dir, "sbl_launch"+strconv.Itoa(bits)+".exe"), args...), nil
}

// parseLauncherOutput reads the launcher's "PID <n>" / "ERR <message>" reply.
func parseLauncherOutput(out string) (uint32, error) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "PID ") {
			n, err := strconv.ParseUint(strings.TrimSpace(line[4:]), 10, 32)
			if err == nil && n > 0 {
				return uint32(n), nil
			}
		}
		if strings.HasPrefix(line, "ERR ") {
			return 0, errors.New(strings.TrimSpace(line[4:]))
		}
	}
	return 0, errors.New("the locale engine gave no answer")
}

// customLocaleArgs expands the user's argument template; {exe} is the game. With no template
// the game is simply the first argument, as in "emulator.exe game.exe [args]".
func customLocaleArgs(tmpl, exe string, gameArgs []string) []string {
	var args []string
	used := false
	for _, t := range splitArgs(tmpl) {
		if strings.Contains(t, "{exe}") {
			used = true
		}
		args = append(args, strings.ReplaceAll(t, "{exe}", exe))
	}
	if !used {
		args = append(args, exe)
	}
	for _, a := range gameArgs {
		args = append(args, quoteArg(a))
	}
	return args
}
