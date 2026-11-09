package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Config struct {
	PresentMon   string   `json:"presentmon"`
	LHMURL       string   `json:"lhm_url"`
	LHMFile      string   `json:"lhm_file"`
	Games        []string `json:"games"`
	GameRoots    []string `json:"game_roots"`
	AutoDiscover bool     `json:"auto_discover"`
	GPUIndex     uint32   `json:"gpu_index"`
}

func normalizedPath(s string) string {
	return strings.TrimRight(strings.ToLower(strings.ReplaceAll(s, "/", `\`)), `\`)
}
func configFileToLoad(path string, explicit bool) (string, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) && !explicit {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return path, nil
}
func exeBase(s string) string { p := strings.Split(normalizedPath(s), `\`); return p[len(p)-1] }
func (c Config) IsGame(path string) bool {
	path = normalizedPath(path)
	name := exeBase(path)
	for _, g := range c.Games {
		if strings.EqualFold(g, name) || normalizedPath(g) == path {
			return true
		}
	}
	for _, s := range []string{"chrome", "browser", "webview", "webhelper", "launcher", "crashreport", "crashhandler", "crashpad", "reporter", "unins", "setup", "updater", "anticheat", "bootstrap", "easyanticheat", "battleye", "helper", "cefsubprocess", "cefsharp"} {
		if strings.Contains(name, s) {
			return false
		}
	}
	if name == "steam.exe" || name == "epicgameslauncher.exe" || name == "ubisoftconnect.exe" || name == "eadesktop.exe" {
		return false
	}
	for _, root := range c.GameRoots {
		if root != "" && strings.HasPrefix(path, normalizedPath(root)+`\`) {
			return true
		}
	}
	return false
}
func loadConfig(path string) (Config, error) {
	c := Config{LHMFile: filepath.Join(os.Getenv("ProgramData"), "SteelSeriesOLED", "sensors.json"), AutoDiscover: true}
	if path != "" {
		b, e := os.ReadFile(path)
		if e != nil {
			return c, e
		}
		if e = json.Unmarshal(b, &c); e != nil {
			return c, e
		}
	}
	if c.PresentMon == "" {
		exe, _ := os.Executable()
		for _, dir := range []string{filepath.Dir(exe), filepath.Join(os.Getenv("USERPROFILE"), "bin")} {
			matches, _ := filepath.Glob(filepath.Join(dir, "PresentMon*-x64.exe"))
			if len(matches) > 0 {
				c.PresentMon = matches[len(matches)-1]
				break
			}
		}
	}
	if c.AutoDiscover {
		c.GameRoots = append(c.GameRoots, discoverGameRoots()...)
	}
	if c.LHMURL != "" && !strings.HasPrefix(c.LHMURL, "http://127.0.0.1:") && !strings.HasPrefix(c.LHMURL, "http://localhost:") {
		return c, fmt.Errorf("lhm_url must use loopback HTTP")
	}
	return c, nil
}

var vdfPath = regexp.MustCompile(`"path"\s+"([^"]+)"`)
var installDir = regexp.MustCompile(`"installdir"\s+"([^"]+)"`)

func discoverGameRoots() []string {
	libraries := []string{filepath.Join(os.Getenv("ProgramFiles(x86)"), "Steam")}
	libraries = append(libraries, steamPath())
	for _, root := range append([]string(nil), libraries...) {
		b, _ := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"))
		for _, m := range vdfPath.FindAllStringSubmatch(string(b), -1) {
			libraries = append(libraries, strings.ReplaceAll(m[1], `\\`, `\`))
		}
	}
	roots := []string{}
	seen := map[string]bool{}
	for _, library := range libraries {
		files, _ := filepath.Glob(filepath.Join(library, "steamapps", "appmanifest_*.acf"))
		for _, f := range files {
			b, _ := os.ReadFile(f)
			m := installDir.FindStringSubmatch(string(b))
			if len(m) != 2 {
				continue
			}
			p := filepath.Join(library, "steamapps", "common", m[1])
			if !seen[normalizedPath(p)] {
				roots = append(roots, p)
				seen[normalizedPath(p)] = true
			}
		}
	}
	manifests, _ := filepath.Glob(filepath.Join(os.Getenv("ProgramData"), "Epic", "EpicGamesLauncher", "Data", "Manifests", "*.item"))
	for _, f := range manifests {
		var m struct{ InstallLocation string }
		b, _ := os.ReadFile(f)
		if json.Unmarshal(b, &m) == nil && m.InstallLocation != "" {
			roots = append(roots, m.InstallLocation)
		}
	}
	roots = append(roots, gogGameRoots()...)
	return roots
}
