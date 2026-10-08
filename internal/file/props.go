package file

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ParametersFile is the externalised configuration of an integration flow.
const ParametersFile = "src/main/resources/parameters.prop"

// propEntry is one logical line of a properties file (continuation lines
// included) with its key.
type propEntry struct {
	key   string
	lines []string
}

// splitProperties returns the logical lines of a properties file; comment
// and blank lines have an empty key and isComment set.
func splitProperties(data string) (entries []propEntry, comments []string) {
	lines := strings.Split(strings.TrimSuffix(data, "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimLeft(line, " \t\f")
		if trimmed == "" || trimmed[0] == '#' || trimmed[0] == '!' {
			comments = append(comments, line)
			continue
		}
		e := propEntry{key: propKey(trimmed), lines: []string{line}}
		for continues(lines[i]) && i+1 < len(lines) {
			i++
			e.lines = append(e.lines, lines[i])
		}
		entries = append(entries, e)
	}
	return entries, comments
}

// continues reports whether a line ends with an odd number of backslashes.
func continues(line string) bool {
	n := 0
	for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// propKey is the raw (escaped) key: up to the first unescaped '=', ':' or
// whitespace.
func propKey(line string) string {
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '=', ':', ' ', '\t', '\f':
			return line[:i]
		}
	}
	return line
}

// NormalizeProperties makes a parameters.prop stable between downloads, as
// the earlier importPackages did: comment lines that contain ':' (the
// timestamp java.util.Properties writes, "#Thu Oct 08 14:11:02 UTC 2026")
// and blank lines are dropped, other comment lines (such as "#") are kept
// at the top in their order, and the entries are sorted by key (stable, so
// duplicate keys keep their order). Lines end with LF.
func NormalizeProperties(data []byte) []byte {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	entries, comments := splitProperties(text)
	var out []string
	for _, c := range comments {
		if strings.TrimSpace(c) == "" || strings.Contains(c, ":") {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
	for _, e := range entries {
		out = append(out, e.lines...)
	}
	if len(out) == 0 {
		return nil
	}
	return []byte(strings.Join(out, "\n") + "\n")
}

// PropertiesEqual compares two properties files by their entries,
// regardless of order, comments and blank lines.
func PropertiesEqual(a, b []byte) bool {
	norm := func(d []byte) []string {
		entries, _ := splitProperties(strings.ReplaceAll(string(d), "\r\n", "\n"))
		out := make([]string, 0, len(entries))
		for _, e := range entries {
			out = append(out, strings.Join(normalizeLines([]byte(strings.Join(e.lines, "\n")), ""), "\n"))
		}
		sort.Strings(out)
		return out
	}
	return equalLines(norm(a), norm(b))
}

// NormalizeParametersFile rewrites dir/src/main/resources/parameters.prop
// with NormalizeProperties (no file: nothing to do).
func NormalizeParametersFile(dir string) error {
	p := filepath.Join(dir, filepath.FromSlash(ParametersFile))
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	norm := NormalizeProperties(data)
	if bytes.Equal(norm, data) {
		return nil
	}
	return os.WriteFile(p, norm, 0o644)
}

// RestoreProperties returns tenant with the entries of keys taken from local
// where local has them (a key local lacks keeps the tenant's value, e.g. on
// the first snapshot). It undoes configOverrides that a deployment merged
// into parameters.prop. The result is normalized.
func RestoreProperties(tenant, local []byte, keys map[string]bool) []byte {
	te, tc := splitProperties(strings.ReplaceAll(string(tenant), "\r\n", "\n"))
	le, _ := splitProperties(strings.ReplaceAll(string(local), "\r\n", "\n"))
	fromLocal := map[string][]string{}
	for _, e := range le {
		if keys[e.key] {
			fromLocal[e.key] = append(fromLocal[e.key], e.lines...)
		}
	}
	out := append([]string{}, tc...)
	for _, e := range te {
		if _, ok := fromLocal[e.key]; !ok {
			out = append(out, e.lines...)
		}
	}
	for _, lines := range fromLocal {
		out = append(out, lines...)
	}
	return NormalizeProperties([]byte(strings.Join(out, "\n") + "\n"))
}
