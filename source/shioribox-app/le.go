package main

import (
	"debug/pe"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Japanese-locale launching works by running a locale emulator that the user installs
// themselves (Locale Emulator's LEProc.exe) as a separate program, exactly as if they had
// started it by hand. ShioriBox contains none of its code; it only finds the program,
// reads which profile is for ja-JP, and runs it with the game's path.

type LEStatus struct {
	Available bool   `json:"available"`
	Dir       string `json:"dir"`     // folder holding LEProc.exe (or where we looked)
	Auto      bool   `json:"auto"`    // found automatically rather than configured
	Profile   string `json:"profile"` // name of the ja-JP profile that will be used
	Problem   string `json:"problem"`
}

const leExeName = "LEProc.exe"

type leProfile struct {
	Name, Guid, Location string
}

type leConfigXML struct {
	Profiles struct {
		Items []struct {
			Name     string `xml:"Name,attr"`
			Guid     string `xml:"Guid,attr"`
			Location string `xml:"Location"`
		} `xml:",any"`
	} `xml:"Profiles"`
}

// readJapaneseProfile reads LEConfig.xml next to LEProc.exe and returns its ja-JP profile, if any.
func readJapaneseProfile(dir string) (leProfile, bool) {
	b, err := os.ReadFile(filepath.Join(dir, "LEConfig.xml"))
	if err != nil {
		return leProfile{}, false
	}
	b = []byte(strings.TrimPrefix(string(b), "\ufeff"))
	var cfg leConfigXML
	if xml.Unmarshal(b, &cfg) != nil {
		return leProfile{}, false
	}
	for _, p := range cfg.Profiles.Items {
		if strings.EqualFold(strings.TrimSpace(p.Location), "ja-JP") && strings.TrimSpace(p.Guid) != "" {
			return leProfile{Name: p.Name, Guid: strings.TrimSpace(p.Guid), Location: "ja-JP"}, true
		}
	}
	return leProfile{}, false
}

func hasLE(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, leExeName))
	return err == nil && !st.IsDir()
}

// findInTree looks for LEProc.exe at most depth levels below root.
func findInTree(root string, depth int) string {
	if hasLE(root) {
		return root
	}
	if depth == 0 {
		return ""
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			if d := findInTree(filepath.Join(root, e.Name()), depth-1); d != "" {
				return d
			}
		}
	}
	return ""
}

func leSearchRoots() (roots []string, depths []int) {
	add := func(p string, d int) {
		if p != "" {
			roots = append(roots, p)
			depths = append(depths, d)
		}
	}
	if exe, err := os.Executable(); err == nil {
		ed := filepath.Dir(exe)
		add(filepath.Join(ed, "LocaleEmulator"), 1)
		add(filepath.Join(ed, "Locale Emulator"), 1)
		add(ed, 2)
	}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
		if b := os.Getenv(env); b != "" {
			add(filepath.Join(b, "Locale Emulator"), 1)
			add(filepath.Join(b, "LocaleEmulator"), 1)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, "Downloads"), 2)
		add(filepath.Join(home, "Desktop"), 2)
	}
	return
}

// leStatus works out whether a locale emulator can be used right now.
func leStatus(st Settings) LEStatus {
	s := LEStatus{}
	if st.LEPath != "" {
		s.Dir = st.LEPath
		if !hasLE(st.LEPath) {
			s.Problem = leExeName + " was not found in that folder."
			return s
		}
	} else {
		roots, depths := leSearchRoots()
		for i, r := range roots {
			if d := findInTree(r, depths[i]); d != "" {
				s.Dir, s.Auto = d, true
				break
			}
		}
		if s.Dir == "" {
			s.Problem = "Locale Emulator was not found on this PC."
			return s
		}
	}
	s.Available = true
	if p, ok := readJapaneseProfile(s.Dir); ok {
		s.Profile = p.Name
	}
	return s
}

// quoteArg wraps an argument containing spaces: the emulator joins arguments with plain spaces.
func quoteArg(a string) string {
	if strings.ContainsAny(a, " \t") && !strings.HasPrefix(a, `"`) {
		return `"` + a + `"`
	}
	return a
}

// leCommand returns the program and arguments that start exe (with its own arguments) in Japanese locale.
func leCommand(st LEStatus, exe string, gameArgs []string) (string, []string) {
	prog := filepath.Join(st.Dir, leExeName)
	var args []string
	if p, ok := readJapaneseProfile(st.Dir); ok {
		args = []string{"-runas", p.Guid, exe}
	} else {
		// no config file yet: the emulator's built-in default profile is ja-JP
		args = []string{exe}
	}
	for _, a := range gameArgs {
		args = append(args, quoteArg(a))
	}
	return prog, args
}

// exeBits tells whether a Windows executable is 32- or 64-bit (0 = unknown).
func exeBits(path string) int {
	f, err := pe.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	switch f.Machine {
	case pe.IMAGE_FILE_MACHINE_I386:
		return 32
	case pe.IMAGE_FILE_MACHINE_AMD64, pe.IMAGE_FILE_MACHINE_ARM64:
		return 64
	}
	return 0
}

func checkLEGame(exe string) error {
	if exeBits(exe) == 64 {
		return fmt.Errorf("this game is 64-bit, and locale emulators only support 32-bit games — turn off “Japanese locale” for it")
	}
	return nil
}
