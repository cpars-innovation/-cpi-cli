package iflow

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileLayout is the outcome for one .iflw file.
type FileLayout struct {
	Path     string `json:"path"`
	Changed  bool   `json:"changed"`
	Moved    int    `json:"moved,omitempty"`
	Rerouted int    `json:"rerouted,omitempty"`
	Added    int    `json:"added,omitempty"`
	// Issues of the diagram as it was (check: as it is).
	Issues []LayoutIssue `json:"issues,omitempty"`
	Error  string        `json:"error,omitempty"`
}

// LayoutReport is the outcome of LayoutPaths.
type LayoutReport struct {
	Mode   string       `json:"mode"`
	DryRun bool         `json:"dryRun,omitempty"`
	Check  bool         `json:"check,omitempty"`
	Files  []FileLayout `json:"files"`
	// Changed files (check: files with issues), Issues found, Failed files.
	Changed int `json:"changed"`
	Issues  int `json:"issues"`
	Failed  int `json:"failed"`
}

// ModelFiles returns the .iflw files of paths: files as given, directories
// searched recursively (.git skipped), sorted.
func ModelFiles(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
			continue
		}
		err = filepath.WalkDir(p, func(f string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			if !d.IsDir() && strings.HasSuffix(d.Name(), ".iflw") && !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

// LayoutPaths lays out (or with check only checks) the .iflw files of
// paths; dryRun computes the layout without writing.
func LayoutPaths(paths []string, o LayoutOptions, dryRun, check bool) (*LayoutReport, error) {
	o, err := o.Normalized()
	if err != nil {
		return nil, err
	}
	files, err := ModelFiles(paths)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .iflw files in %s", strings.Join(paths, ", "))
	}
	rep := &LayoutReport{Mode: o.Mode, DryRun: dryRun, Check: check, Files: []FileLayout{}}
	for _, f := range files {
		fl := FileLayout{Path: filepath.ToSlash(f)}
		err := func() error {
			data, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			if fl.Issues, err = CheckLayout(data); err != nil {
				return err
			}
			rep.Issues += len(fl.Issues)
			if check {
				fl.Changed = len(fl.Issues) > 0
				return nil
			}
			out, res, err := Layout(data, o)
			if err != nil {
				return err
			}
			fl.Changed, fl.Moved, fl.Rerouted, fl.Added = res.Changed, res.Moved, res.Rerouted, res.Added
			if res.Changed && !dryRun {
				return os.WriteFile(f, out, 0o644)
			}
			return nil
		}()
		if err != nil {
			fl.Error = err.Error()
			rep.Failed++
		}
		if fl.Changed {
			rep.Changed++
		}
		rep.Files = append(rep.Files, fl)
	}
	return rep, nil
}
