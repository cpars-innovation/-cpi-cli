package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	gosync "sync"
	"time"

	"github.com/cpars-innovation/cpicli/internal/file"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
)

// SnapshotState remembers per artifact what the last snapshot saw on the
// tenant and wrote locally, so that an incremental snapshot can skip the
// download of artifacts that did not change.
type SnapshotState struct {
	mu        gosync.Mutex
	Format    int                      `json:"format"`
	Artifacts map[string]ArtifactState `json:"artifacts"` // key "<package>/<artifact>"
}

// ArtifactState is the signature of one artifact.
type ArtifactState struct {
	Type    string `json:"type"`
	Version string `json:"version"`
	// ModifiedAt is the tenant's last change (zero: not reported, the
	// artifact is never skipped).
	ModifiedAt time.Time `json:"modifiedAt"`
	// ConfigHash is the SHA-256 of the configured parameters (integration
	// flows): Configure changes need not change version or ModifiedAt.
	ConfigHash string `json:"configHash,omitempty"`
	// ContentHash is the SHA-256 of the local directory after the snapshot
	// (META-INF and src/main/resources): a changed or missing local copy is
	// downloaded again.
	ContentHash string `json:"contentHash"`
}

// LoadSnapshotState reads the state file; a missing file is an empty state.
func LoadSnapshotState(path string) (*SnapshotState, error) {
	st := &SnapshotState{Format: 1, Artifacts: map[string]ArtifactState{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, st); err != nil {
		return nil, err
	}
	if st.Artifacts == nil {
		st.Artifacts = map[string]ArtifactState{}
	}
	return st, nil
}

// Save writes the state file (sorted, indented, for readable diffs).
func (st *SnapshotState) Save(path string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ") // map keys are sorted
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (st *SnapshotState) get(key string) (ArtifactState, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	a, ok := st.Artifacts[key]
	return a, ok
}

func (st *SnapshotState) set(key string, a ArtifactState) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.Artifacts[key] = a
}

// tenantSignature is what the tenant says about an artifact without a
// download: version and ModifiedAt from the listing, plus the hash of the
// configured parameters for integration flows.
func tenantSignature(exe *httpclnt.HTTPExecuter, a *cpi.ArtifactDetails) (ArtifactState, error) {
	sig := ArtifactState{Type: a.ArtifactType, Version: a.Version, ModifiedAt: a.ModifiedAt.UTC()}
	if a.ArtifactType != "Integration" {
		return sig, nil
	}
	params, err := cpi.NewConfiguration(exe).Get(a.Id, "active")
	if err != nil {
		return sig, err
	}
	lines := make([]string, 0, len(params.Root.Results))
	for _, p := range params.Root.Results {
		lines = append(lines, p.ParameterKey+"\x00"+p.ParameterValue+"\x00"+p.DataType)
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{'\n'})
	}
	sig.ConfigHash = hex.EncodeToString(h.Sum(nil))
	return sig, nil
}

// unchangedSince reports whether sig equals the recorded state and the local
// copy is still what the last snapshot wrote.
func unchangedSince(prev, sig ArtifactState, localDir string) bool {
	if sig.ModifiedAt.IsZero() || prev.ModifiedAt.IsZero() {
		return false // without a modification time nothing is skipped
	}
	if prev.Type != sig.Type || prev.Version != sig.Version || !prev.ModifiedAt.Equal(sig.ModifiedAt) || prev.ConfigHash != sig.ConfigHash {
		return false
	}
	hash, err := localContentHash(localDir)
	return err == nil && hash != "" && hash == prev.ContentHash
}

func localContentHash(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "META-INF", "MANIFEST.MF")); err != nil {
		return "", err
	}
	return file.ContentHash(os.DirFS(dir))
}
