package ops

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
)

// Parameter is an externalised configuration parameter of an artifact.
type Parameter struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	DataType string `json:"dataType,omitempty"`
}

// GetConfiguration returns the configuration parameters of a designtime
// artifact, sorted by key. version defaults to "active".
func GetConfiguration(exe *httpclnt.HTTPExecuter, artifactID, version string) ([]Parameter, error) {
	if version == "" {
		version = "active"
	}
	data, err := cpi.NewConfiguration(exe).Get(artifactID, version)
	if err != nil {
		return nil, err
	}
	params := make([]Parameter, 0, len(data.Root.Results))
	for _, p := range data.Root.Results {
		params = append(params, Parameter{Key: p.ParameterKey, Value: p.ParameterValue, DataType: p.DataType})
	}
	sort.Slice(params, func(i, j int) bool { return params[i].Key < params[j].Key })
	return params, nil
}

// ParameterFailure is a parameter that could not be updated.
type ParameterFailure struct {
	Key   string `json:"key"`
	Error string `json:"error"`
}

// ConfigurationUpdate is the result of UpdateConfiguration.
type ConfigurationUpdate struct {
	ArtifactID string             `json:"artifactId"`
	DryRun     bool               `json:"dryRun"`
	Updated    []string           `json:"updated"`
	Unchanged  []string           `json:"unchanged"`
	NotFound   []string           `json:"notFound"`
	Failed     []ParameterFailure `json:"failed"`
}

// UpdateConfiguration sets the given parameters of a designtime artifact.
// Only parameters whose value differs are written. Unknown keys are reported
// in NotFound and make the call fail (as a usage error) before anything is
// written, so a typo never results in a half-applied configuration.
// Deploy the artifact afterwards for the change to take effect at runtime.
func UpdateConfiguration(exe *httpclnt.HTTPExecuter, artifactID, version string, values map[string]string, dryRun bool) (*ConfigurationUpdate, error) {
	if version == "" {
		version = "active"
	}
	current, err := GetConfiguration(exe, artifactID, version)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]string, len(current))
	for _, p := range current {
		existing[p.Key] = p.Value
	}

	res := &ConfigurationUpdate{ArtifactID: artifactID, DryRun: dryRun,
		Updated: []string{}, Unchanged: []string{}, NotFound: []string{}, Failed: []ParameterFailure{}}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var toUpdate []string
	for _, k := range keys {
		old, ok := existing[k]
		switch {
		case !ok:
			res.NotFound = append(res.NotFound, k)
		case old == values[k]:
			res.Unchanged = append(res.Unchanged, k)
		default:
			toUpdate = append(toUpdate, k)
		}
	}
	if len(res.NotFound) > 0 {
		return res, output.Usagef("unknown parameter(s) for artifact %s: %s", artifactID, strings.Join(res.NotFound, ", "))
	}
	if dryRun {
		res.Updated = append(res.Updated, toUpdate...)
		return res, nil
	}

	cfg := cpi.NewConfiguration(exe)
	var firstErr error
	for _, k := range toUpdate {
		if err := cfg.Update(artifactID, version, k, values[k]); err != nil {
			res.Failed = append(res.Failed, ParameterFailure{Key: k, Error: err.Error()})
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		res.Updated = append(res.Updated, k)
	}
	if len(res.Failed) > 0 {
		err := fmt.Errorf("%d of %d parameter update(s) failed for %s: %w", len(res.Failed), len(toUpdate), artifactID, firstErr)
		if len(res.Updated) > 0 {
			return res, output.Partial(err)
		}
		return res, err
	}
	return res, nil
}
