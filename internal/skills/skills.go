// Package skills lists, reads and installs the cpi skills embedded in cpictl.
package skills

import (
	"bufio"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/plugin"
)

// Skill is an embedded skill.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Files are the skill's files relative to its folder (SKILL.md first).
	Files []string `json:"files"`
}

// Agent is an embedded subagent (Claude Code only).
type Agent struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// List returns the embedded skills, sorted by name.
func List() []Skill {
	entries, _ := fs.ReadDir(plugin.FS, "skills")
	var res []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(plugin.FS, path.Join("skills", e.Name(), "SKILL.md"))
		if err != nil {
			continue
		}
		fm := frontmatter(string(data))
		s := Skill{Name: firstNonEmpty(fm["name"], e.Name()), Description: fm["description"], Files: []string{"SKILL.md"}}
		_ = fs.WalkDir(plugin.FS, path.Join("skills", e.Name()), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && path.Base(p) != "SKILL.md" {
				s.Files = append(s.Files, strings.TrimPrefix(p, "skills/"+e.Name()+"/"))
			}
			return nil
		})
		sort.Strings(s.Files[1:])
		res = append(res, s)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Name < res[j].Name })
	return res
}

// Agents returns the embedded subagents.
func Agents() []Agent {
	entries, _ := fs.ReadDir(plugin.FS, "agents")
	var res []Agent
	for _, e := range entries {
		data, err := fs.ReadFile(plugin.FS, path.Join("agents", e.Name()))
		if err != nil || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		fm := frontmatter(string(data))
		res = append(res, Agent{Name: firstNonEmpty(fm["name"], strings.TrimSuffix(e.Name(), ".md")), Description: fm["description"]})
	}
	return res
}

// Find returns the skill with this name.
func Find(name string) (Skill, bool) {
	for _, s := range List() {
		if s.Name == name {
			return s, true
		}
	}
	return Skill{}, false
}

// Read returns a file of a skill (default SKILL.md).
func Read(name, file string) (string, error) {
	s, ok := Find(name)
	if !ok {
		return "", output.Usagef("no skill %q (skills: %s)", name, strings.Join(Names(), ", "))
	}
	if file == "" {
		file = "SKILL.md"
	}
	if !slices.Contains(s.Files, file) {
		return "", output.Usagef("skill %s has no file %q (files: %s)", name, file, strings.Join(s.Files, ", "))
	}
	data, err := fs.ReadFile(plugin.FS, path.Join("skills", name, file))
	return string(data), err
}

// Names returns the skill names.
func Names() []string {
	var names []string
	for _, s := range List() {
		names = append(names, s.Name)
	}
	return names
}

// Targets are the skill folders of the agents, relative to a repository or
// the home directory.
var Targets = map[string]string{
	"agents": ".agents/skills", // Codex, Cursor and others reading the shared location
	"codex":  ".agents/skills",
	"cursor": ".cursor/skills",
	"gemini": ".gemini/skills",
	"claude": ".claude/skills", // Claude Code without the plugin
}

// AgentNames lists the keys of Targets.
func AgentNames() []string {
	names := make([]string, 0, len(Targets))
	for k := range Targets {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// InstallResult lists the installed skills.
type InstallResult struct {
	Dir    string   `json:"dir"`
	Skills []string `json:"skills"`
}

var reAllowedTools = regexp.MustCompile(`(?m)^allowed-tools:.*\n`)

// Install writes all skills into base/<agent folder>, replacing existing
// cpi skills there and keeping other skills. The allowed-tools line is
// removed: it names the tools as the Claude Code plugin sees them.
func Install(agent, base string) (*InstallResult, error) {
	rel, ok := Targets[agent]
	if !ok {
		return nil, output.Usagef("unknown agent %q (%s)", agent, strings.Join(AgentNames(), ", "))
	}
	dir := filepath.Join(base, filepath.FromSlash(rel))
	res := &InstallResult{Dir: dir}
	for _, s := range List() {
		target := filepath.Join(dir, s.Name)
		if err := os.RemoveAll(target); err != nil {
			return nil, err
		}
		for _, f := range s.Files {
			data, err := fs.ReadFile(plugin.FS, path.Join("skills", s.Name, f))
			if err != nil {
				return nil, err
			}
			if f == "SKILL.md" {
				data = reAllowedTools.ReplaceAll(data, nil)
			}
			p := filepath.Join(target, filepath.FromSlash(f))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(p, data, 0o644); err != nil {
				return nil, err
			}
		}
		res.Skills = append(res.Skills, s.Name)
	}
	return res, nil
}

// frontmatter reads the "key: value" lines between the leading "---" lines.
func frontmatter(doc string) map[string]string {
	res := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(doc))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return res
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			res[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return res
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
