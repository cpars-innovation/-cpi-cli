package skills

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedSkills(t *testing.T) {
	// every skill folder of the plugin is embedded and described
	dirs, err := os.ReadDir("../../plugin/skills")
	require.NoError(t, err)
	var want []string
	for _, d := range dirs {
		if d.IsDir() {
			want = append(want, d.Name())
		}
	}
	assert.Equal(t, want, Names())
	for _, s := range List() {
		assert.NotEmpty(t, s.Description, s.Name)
		assert.Equal(t, "SKILL.md", s.Files[0], s.Name)
	}
	build, ok := Find("cpi-build")
	require.True(t, ok)
	assert.Contains(t, build.Files, "iflow-structure.md")
	require.Len(t, Agents(), 1)
	assert.Equal(t, "cpi-reviewer", Agents()[0].Name)

	text, err := Read("cpi-plan", "brief-template.md")
	require.NoError(t, err)
	assert.Contains(t, text, "# Brief")
	_, err = Read("cpi-plan", "../cpi-build/SKILL.md")
	assert.Error(t, err)
	_, err = Read("nope", "")
	assert.ErrorContains(t, err, "cpi-discover")
}

func TestInstall(t *testing.T) {
	base := t.TempDir()
	other := filepath.Join(base, ".agents", "skills", "my-skill", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(other), 0o755))
	require.NoError(t, os.WriteFile(other, []byte("mine"), 0o644))
	stale := filepath.Join(base, ".agents", "skills", "cpi-build", "old.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(stale), 0o755))
	require.NoError(t, os.WriteFile(stale, []byte("old"), 0o644))

	res, err := Install("codex", base, false)
	require.NoError(t, err)
	assert.Equal(t, Names(), res.Skills)
	data, err := os.ReadFile(filepath.Join(base, ".agents", "skills", "cpi-discover", "SKILL.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "allowed-tools")
	assert.Contains(t, string(data), "name: cpi-discover")
	assert.FileExists(t, filepath.Join(base, ".agents", "skills", "cpi-plan", "brief-template.md"))
	assert.FileExists(t, other, "other skills are kept")
	assert.NoFileExists(t, stale, "cpi skills are replaced")

	_, err = Install("vim", base, false)
	assert.Error(t, err)

	// OpenCode: .opencode/skills in a repository, ~/.config/opencode/skills for the user
	res, err = Install("opencode", base, false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, ".opencode", "skills"), res.Dir)
	res, err = Install("opencode", base, true)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, ".config", "opencode", "skills"), res.Dir)
	assert.FileExists(t, filepath.Join(res.Dir, "cpi-build", "SKILL.md"))
}

// Agent Skills rules (OpenCode enforces them): lowercase names matching the
// folder, descriptions of 1 to 1024 characters.
func TestSkillsFollowTheAgentSkillsFormat(t *testing.T) {
	name := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	for _, s := range List() {
		assert.Regexp(t, name, s.Name)
		assert.LessOrEqual(t, len(s.Name), 64, s.Name)
		assert.NotEmpty(t, s.Description, s.Name)
		assert.LessOrEqual(t, len(s.Description), 1024, s.Name)
	}
}
