// Package stats keeps local usage statistics: one line per command or MCP
// tool call (name, source, exit code, duration, time) in
// $HOME/.cpictl/stats.jsonl. Nothing else is recorded (no arguments, hosts
// or artifact names) and nothing is sent anywhere. CPICTL_STATS=off turns it
// off; the file is compacted to its newest half when it exceeds MaxBytes.
package stats

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// MaxBytes is the size at which the file is compacted (about 10,000 entries).
const MaxBytes = 1 << 20

// Sources of an entry.
const (
	SourceCLI = "cli"
	SourceMCP = "mcp"
)

// Entry is one recorded run.
type Entry struct {
	Time     time.Time `json:"t"`
	Command  string    `json:"cmd"`
	Source   string    `json:"src"`
	ExitCode int       `json:"exit"`
	Ms       int64     `json:"ms"`
}

// Path is the stats file: $CPICTL_STATS_FILE, else $HOME/.cpictl/stats.jsonl.
// Empty when stats are off: CPICTL_STATS=off (or 0, false), or under
// go test without CPICTL_STATS_FILE.
func Path() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CPICTL_STATS"))) {
	case "off", "0", "false", "no":
		return ""
	}
	if p := os.Getenv("CPICTL_STATS_FILE"); p != "" {
		return p
	}
	if testing.Testing() {
		return "" // tests never write to the real home directory
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cpictl", "stats.jsonl")
}

// Record appends one entry. Errors are ignored: statistics never make a
// command fail (read-only home directories, CI runners).
func Record(command, source string, exitCode int, d time.Duration) {
	path := Path()
	if path == "" || command == "" {
		return
	}
	line, err := json.Marshal(Entry{Time: time.Now().UTC().Truncate(time.Second), Command: command, Source: source, ExitCode: exitCode, Ms: d.Milliseconds()})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n')) // one small O_APPEND write: safe across processes
	info, err := f.Stat()
	_ = f.Close()
	if err == nil && info.Size() > MaxBytes {
		_ = compact(path)
	}
}

// compact keeps the newest half of the file. Concurrent writers may lose an
// entry written during the rewrite; the file never grows past ~MaxBytes.
func compact(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	keep := data[len(data)-MaxBytes/2:]
	if i := bytes.IndexByte(keep, '\n'); i >= 0 {
		keep = keep[i+1:]
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".stats-*.jsonl")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(keep); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Read returns the entries since the given time (zero: all). Unreadable
// lines are skipped.
func Read(path string, since time.Time) ([]Entry, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), 64*1024)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Command == "" {
			continue
		}
		if !since.IsZero() && e.Time.Before(since) {
			continue
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// Row is the summary of one command.
type Row struct {
	Command  string    `json:"command"`
	Source   string    `json:"source"`
	Runs     int       `json:"runs"`
	Failed   int       `json:"failed"`
	P50Ms    int64     `json:"p50Ms"`
	P95Ms    int64     `json:"p95Ms"`
	MaxMs    int64     `json:"maxMs"`
	TotalMs  int64     `json:"totalMs"`
	LastUsed time.Time `json:"lastUsed"`
}

// Summarize groups entries by source and command, most used first.
func Summarize(entries []Entry) []Row {
	type key struct{ src, cmd string }
	groups := map[key][]Entry{}
	for _, e := range entries {
		k := key{e.Source, e.Command}
		groups[k] = append(groups[k], e)
	}
	rows := make([]Row, 0, len(groups))
	for k, es := range groups {
		r := Row{Command: k.cmd, Source: k.src, Runs: len(es)}
		ms := make([]int64, 0, len(es))
		for _, e := range es {
			if e.ExitCode != 0 {
				r.Failed++
			}
			ms = append(ms, e.Ms)
			r.TotalMs += e.Ms
			if e.Time.After(r.LastUsed) {
				r.LastUsed = e.Time
			}
		}
		slices.Sort(ms)
		r.P50Ms, r.P95Ms, r.MaxMs = percentile(ms, 50), percentile(ms, 95), ms[len(ms)-1]
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Runs != rows[j].Runs {
			return rows[i].Runs > rows[j].Runs
		}
		if rows[i].Source != rows[j].Source {
			return rows[i].Source < rows[j].Source
		}
		return rows[i].Command < rows[j].Command
	})
	return rows
}

// percentile of sorted values (nearest rank).
func percentile(sorted []int64, p int) int64 {
	i := (p*len(sorted)+99)/100 - 1
	return sorted[max(0, min(i, len(sorted)-1))]
}
