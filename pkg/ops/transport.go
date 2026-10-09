package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/cpars-innovation/cpicli/internal/file"
	"github.com/cpars-innovation/cpicli/internal/models"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/cpars-innovation/cpicli/pkg/iflow"
)

// Dependency kinds of a transport.
const (
	DepArtifact   = "artifact"   // script collection, message mapping, value mapping the flow references
	DepFlowCall   = "flow_call"  // a flow the selected flow sends to (ProcessDirect / JMS)
	DepCredential = "credential" // credential name or key alias
	DepPD         = "pd"         // Partner Directory parameter PID:ID
)

// TransportArtifact is an artifact of a transport.
type TransportArtifact struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Package string `json:"package"`
	// Path is the folder relative to the content tree.
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
	// AddedAsDependency marks artifacts added by WithDeps.
	AddedAsDependency bool `json:"addedAsDependency,omitempty"`
}

// TransportDependency is something a selected artifact needs on the target.
type TransportDependency struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	// Type and Package of artifact and flow_call dependencies.
	Type    string `json:"type,omitempty"`
	Package string `json:"package,omitempty"`
	// By are the selected artifacts that need it, Reason how.
	By     []string `json:"by"`
	Reason string   `json:"reason,omitempty"`
	// InSelection: part of the transport; Local: found in the content tree.
	InSelection bool `json:"inSelection,omitempty"`
	Local       bool `json:"local,omitempty"`
}

// TransportSet is a selection with its dependencies.
type TransportSet struct {
	Dir          string                `json:"dir"`
	Artifacts    []TransportArtifact   `json:"artifacts"`
	Dependencies []TransportDependency `json:"dependencies"`
}

// ResolveTransport selects artifacts of a content tree by ID (or folder
// name) and lists what they depend on: referenced script collections and
// mappings, flows they call, credentials and Partner Directory parameters.
// withDeps adds the referenced artifacts of the tree to the selection
// (transitively); called flows are only reported.
func ResolveTransport(ctx context.Context, dir string, ids []string, withDeps bool) (*TransportSet, error) {
	if len(ids) == 0 {
		return nil, output.Usagef("no artifacts selected")
	}
	local := map[string]LocalArtifact{}
	if err := WalkLocalArtifacts(ctx, dir, func(la LocalArtifact) { local[la.ID] = la }, func(error) {}); err != nil {
		return nil, err
	}
	byFolder := map[string]string{}
	for id, la := range local {
		byFolder[filepath.Base(la.Dir)] = id
	}
	selected := map[string]bool{}
	for _, id := range ids {
		switch {
		case local[id].ID != "":
			selected[id] = true
		case byFolder[id] != "":
			selected[byFolder[id]] = true
		default:
			return nil, output.Usagef("artifact %s not found in %s", id, dir)
		}
	}
	added := map[string]bool{}
	var deps map[string]*TransportDependency
	for {
		var err error
		if deps, err = transportDeps(ctx, dir, local, selected); err != nil {
			return nil, err
		}
		if !withDeps {
			break
		}
		grew := false
		for _, d := range deps {
			if d.Kind == DepArtifact && d.Local && !selected[d.ID] {
				selected[d.ID], added[d.ID], grew = true, true, true
			}
		}
		if !grew {
			break
		}
	}
	set := &TransportSet{Dir: dir, Artifacts: []TransportArtifact{}, Dependencies: []TransportDependency{}}
	for _, id := range sortedKeys(selected) {
		la := local[id]
		v, _ := manifestVersionOf(la.Dir)
		set.Artifacts = append(set.Artifacts, TransportArtifact{ID: id, Type: la.Type, Package: la.PackageID, Path: la.Rel, Version: v, AddedAsDependency: added[id]})
	}
	for _, k := range sortedKeys(deps) {
		d := deps[k]
		d.InSelection = selected[d.ID] && (d.Kind == DepArtifact || d.Kind == DepFlowCall)
		sort.Strings(d.By)
		set.Dependencies = append(set.Dependencies, *d)
	}
	return set, nil
}

// transportDeps lists the dependencies of the selected artifacts.
func transportDeps(ctx context.Context, dir string, local map[string]LocalArtifact, selected map[string]bool) (map[string]*TransportDependency, error) {
	deps := map[string]*TransportDependency{}
	add := func(kind, id, by, reason string) *TransportDependency {
		key := kind + "\x00" + id
		d := deps[key]
		if d == nil {
			d = &TransportDependency{Kind: kind, ID: id, Reason: reason}
			deps[key] = d
		}
		if !slices.Contains(d.By, by) {
			d.By = append(d.By, by)
		}
		return d
	}
	// artifacts referenced in the models: script collections by
	// scriptBundleId, other artifacts by ID in a property value
	referable := map[string]LocalArtifact{}
	for id, la := range local {
		if la.Type != "Integration" {
			referable[id] = la
		}
	}
	for id := range selected {
		la := local[id]
		if la.Type != "Integration" {
			continue
		}
		models, _ := filepath.Glob(filepath.Join(la.Dir, "src", "main", "resources", "scenarioflows", "integrationflow", "*.iflw"))
		for _, m := range models {
			data, err := os.ReadFile(m)
			if err != nil {
				return nil, err
			}
			model, err := iflow.Parse(data)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", m, err)
			}
			for _, el := range model.Elements {
				for key, value := range el.Props {
					value = strings.TrimSpace(value)
					if value == "" {
						continue
					}
					if key == "scriptBundleId" {
						d := add(DepArtifact, value, id, "script collection (step "+cmpOr(el.Name, el.ID)+")")
						d.Type, d.Local = "ScriptCollection", referable[value].ID != ""
						d.Package = referable[value].PackageID
						continue
					}
					for rid, r := range referable {
						if value == rid || strings.Contains(value, "/"+rid+"/") || strings.HasSuffix(value, "/"+rid) {
							d := add(DepArtifact, rid, id, typeLabel(r.Type)+" ("+key+" of "+cmpOr(el.Name, el.ID)+")")
							d.Type, d.Package, d.Local = r.Type, r.PackageID, true
						}
					}
				}
			}
		}
	}
	// calls, credentials, Partner Directory from the discovery
	d, err := DiscoverDir(ctx, dir)
	if err != nil {
		return nil, err
	}
	g := BuildGraph(d)
	for _, f := range d.IFlows {
		if !selected[f.ID] {
			continue
		}
		for _, ref := range f.CredentialRefs {
			if ref != "" {
				add(DepCredential, ref, f.ID, "credential or key alias")
			}
		}
		for _, ref := range f.PDReferences {
			add(DepPD, ref, f.ID, "Partner Directory parameter")
		}
	}
	for _, e := range g.Edges {
		if e.Type != EdgeSendsTo {
			continue
		}
		from, to := strings.TrimPrefix(e.From, NodeIFlow+":"), strings.TrimPrefix(e.To, NodeIFlow+":")
		if !selected[from] || from == to {
			continue
		}
		dep := add(DepFlowCall, to, from, cmpOr(e.Adapter, "call")+" "+strings.TrimPrefix(e.Via, NodeEndpoint+":"))
		dep.Type, dep.Local = "Integration", local[to].ID != ""
		dep.Package = local[to].PackageID
	}
	return deps, nil
}

func typeLabel(t string) string {
	switch t {
	case "MessageMapping":
		return "message mapping"
	case "ValueMapping":
		return "value mapping"
	case "ScriptCollection":
		return "script collection"
	}
	return t
}

// Results of a transport pre-check (and CheckWarn).
const (
	CheckPass = "pass"
	CheckFail = "fail"
	CheckSkip = "skip"
)

// TransportCheck is one pre-check against the target tier.
type TransportCheck struct {
	Artifact string `json:"artifact,omitempty"`
	// Check: draft, exists, drift, dependency, flow_call, credential, pd,
	// parameters.
	Check   string `json:"check"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// TransportCheckOptions configure CheckTransport.
type TransportCheckOptions struct {
	WithDeps bool
	// TargetDir is the target tier's content in Git: an artifact whose
	// tenant content differs from it was changed outside the pipeline.
	TargetDir string
	// Configure is the target tier's configure file: every parameter of
	// parameters.propdef should get a value there, else the source value
	// travels with the artifact.
	Configure *models.ConfigureConfig
}

// TransportCheckResult is the outcome of CheckTransport.
type TransportCheckResult struct {
	*TransportSet
	Checks  []TransportCheck `json:"checks"`
	Summary map[string]int   `json:"summary"`
}

// CheckTransport resolves a selection and checks it against the target
// tenant (read only): drafts, drift, dependencies and called flows on the
// target, credentials and key aliases, Partner Directory parameters and
// parameter values.
func CheckTransport(ctx context.Context, exe *httpclnt.HTTPExecuter, dir string, ids []string, o TransportCheckOptions) (*TransportCheckResult, error) {
	set, err := ResolveTransport(ctx, dir, ids, o.WithDeps)
	if err != nil {
		return nil, err
	}
	res := &TransportCheckResult{TransportSet: set, Checks: []TransportCheck{}, Summary: map[string]int{}}
	add := func(artifact, check, status, format string, args ...any) {
		res.Checks = append(res.Checks, TransportCheck{Artifact: artifact, Check: check, Status: status, Message: fmt.Sprintf(format, args...)})
		res.Summary[status]++
	}
	ip := cpi.NewIntegrationPackage(exe)
	listed := map[string]map[string]*cpi.ArtifactDetails{} // package -> id -> details
	targetArtifact := func(pkg, id string) (*cpi.ArtifactDetails, error) {
		if _, ok := listed[pkg]; !ok {
			listed[pkg] = map[string]*cpi.ArtifactDetails{}
			if _, _, exists, err := ip.Get(pkg); err != nil {
				return nil, err
			} else if exists {
				all, err := ip.GetAllArtifacts(pkg)
				if err != nil {
					return nil, err
				}
				for _, a := range all {
					listed[pkg][a.Id] = a
				}
			}
		}
		return listed[pkg][id], nil
	}
	rt := cpi.NewRuntime(exe)

	// per artifact: draft, existence, drift
	for _, a := range set.Artifacts {
		t, err := targetArtifact(a.Package, a.ID)
		if err != nil {
			return nil, err
		}
		switch {
		case t == nil:
			add(a.ID, "exists", CheckPass, "new on the target (package %s)", a.Package)
			continue
		case t.IsDraft:
			add(a.ID, "draft", CheckFail, "in draft on the target: someone is editing it in the Web UI; save or discard the draft first")
		default:
			add(a.ID, "exists", CheckPass, "on the target with version %s (transport: %s)", t.Version, cmpOr(a.Version, "-"))
		}
		if o.TargetDir == "" {
			continue
		}
		cmpRes, err := Compare(ctx, CompareSide{Label: "target Git", Dir: o.TargetDir}, CompareSide{Label: "target tenant", Exe: exe},
			CompareOptions{Packages: []string{a.Package}, Artifacts: []string{a.ID}, Parallel: 1})
		if err != nil && cmpRes == nil {
			return nil, err
		}
		for _, it := range cmpRes.Items {
			switch it.Status {
			case CompareSame, CompareOnlyB:
				add(a.ID, "drift", CheckPass, "the target tenant matches its Git content")
			case CompareOnlyA:
			default:
				by := ""
				if it.B != nil && it.B.ModifiedBy != "" {
					by = " by " + it.B.ModifiedBy
				}
				add(a.ID, "drift", CheckWarn, "changed on the target outside the pipeline%s (%s): adopt it into Git first, or the transport overwrites it", by, it.Status)
			}
		}
	}

	// dependencies on the target
	selected := map[string]bool{}
	for _, a := range set.Artifacts {
		selected[a.ID] = true
	}
	var credentials map[string]bool
	for _, d := range set.Dependencies {
		by := strings.Join(d.By, ", ")
		switch d.Kind {
		case DepArtifact:
			if d.InSelection {
				add(by, "dependency", CheckPass, "%s %s is part of the transport", typeLabel(d.Type), d.ID)
				continue
			}
			t, err := targetArtifact(d.Package, d.ID)
			if err != nil {
				return nil, err
			}
			if d.Package == "" || t == nil {
				t, err = findTargetArtifact(ip, d.ID, d.Type)
				if err != nil {
					return nil, err
				}
			}
			if t == nil {
				add(by, "dependency", CheckFail, "%s %s is neither on the target nor in the transport", typeLabel(d.Type), d.ID)
			} else {
				add(by, "dependency", CheckPass, "%s %s is on the target (version %s)", typeLabel(d.Type), d.ID, t.Version)
			}
		case DepFlowCall:
			if d.InSelection {
				add(by, "flow_call", CheckPass, "calls %s, which is part of the transport", d.ID)
				continue
			}
			r, err := rt.GetArtifact(d.ID)
			if err != nil {
				return nil, err
			}
			if r == nil || r.Status != "STARTED" {
				add(by, "flow_call", CheckWarn, "calls %s (%s), which is not running on the target", d.ID, d.Reason)
			} else {
				add(by, "flow_call", CheckPass, "calls %s, running on the target (version %s)", d.ID, r.Version)
			}
		case DepCredential:
			if strings.Contains(d.ID, "{{") {
				add(by, "credential", CheckSkip, "%s is a parameter: check the configured name on the target", d.ID)
				continue
			}
			if credentials == nil {
				if credentials, err = targetCredentials(exe); err != nil {
					return nil, err
				}
			}
			if credentials[d.ID] {
				add(by, "credential", CheckPass, "%s exists on the target", d.ID)
			} else {
				add(by, "credential", CheckFail, "%s is neither a credential nor a key alias on the target: deploy it first (cpictl credentials / keystore)", d.ID)
			}
		case DepPD:
			pid, id, _ := strings.Cut(d.ID, ":")
			if pid == "" || id == "" {
				add(by, "pd", CheckSkip, "Partner Directory parameter %s is read with a dynamic partner ID", d.ID)
				continue
			}
			params, err := GetPDParameters(cpi.NewPartnerDirectory(exe), pid, []string{id}, false, 0)
			if err != nil {
				return nil, err
			}
			if len(params.Missing) > 0 {
				add(by, "pd", CheckFail, "Partner Directory parameter %s is missing on the target", d.ID)
			} else {
				add(by, "pd", CheckPass, "Partner Directory parameter %s exists on the target", d.ID)
			}
		}
	}

	// parameter values for the target
	for _, a := range set.Artifacts {
		if a.Type != "Integration" {
			continue
		}
		names, ok, err := file.PropdefNames(filepath.Join(dir, filepath.FromSlash(a.Path)))
		if err != nil {
			return nil, err
		}
		if !ok || len(names) == 0 {
			continue
		}
		if o.Configure == nil {
			add(a.ID, "parameters", CheckSkip, "%d parameter(s): give the target's configure file to check their values", len(names))
			continue
		}
		configured := map[string]bool{}
		for _, p := range o.Configure.Packages {
			for _, art := range p.Artifacts {
				if art.ID == a.ID {
					for _, prm := range art.Parameters {
						configured[prm.Key] = true
					}
				}
			}
		}
		var fromSource []string
		for _, n := range sortedKeys(names) {
			if !configured[n] {
				fromSource = append(fromSource, n)
			}
		}
		if len(fromSource) == 0 {
			add(a.ID, "parameters", CheckPass, "all %d parameter(s) have a value in the target's configure file", len(names))
		} else {
			add(a.ID, "parameters", CheckWarn, "no value in the target's configure file, the source tier's value travels: %s", strings.Join(fromSource, ", "))
		}
	}
	return res, nil
}

// findTargetArtifact looks an artifact up in every package of the target.
func findTargetArtifact(ip *cpi.IntegrationPackage, id, typ string) (*cpi.ArtifactDetails, error) {
	packages, err := ip.GetPackagesList()
	if err != nil {
		return nil, err
	}
	for _, p := range packages {
		arts, err := ip.GetArtifactsData(p, typ)
		if err != nil {
			return nil, err
		}
		if a := cpi.FindArtifactById(id, arts); a != nil {
			return a, nil
		}
	}
	return nil, nil
}

// targetCredentials are the credential names and key aliases of a tenant.
func targetCredentials(exe *httpclnt.HTTPExecuter) (map[string]bool, error) {
	out := map[string]bool{}
	sec, err := ListCredentials(exe, "")
	if err != nil {
		return nil, err
	}
	for _, c := range sec.UserCredentials {
		out[c.Name] = true
	}
	for _, c := range sec.OAuth2Credentials {
		out[c.Name] = true
	}
	for _, c := range sec.SecureParameters {
		out[c.Name] = true
	}
	if ks, err := ListKeystore(exe, "", 0, false, time.Now()); err == nil {
		for _, e := range ks.Entries {
			out[e.Alias] = true
		}
	}
	return out, nil
}

// CopyResult lists what CopyArtifacts did per artifact.
type CopyResult struct {
	DryRun    bool         `json:"dryRun,omitempty"`
	Artifacts []CopiedItem `json:"artifacts"`
}

// CopiedItem is one copied artifact folder.
type CopiedItem struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	// Action: created, updated, unchanged.
	Action string `json:"action"`
}

// CopyArtifacts copies the folders of selected artifacts (with withDeps
// their referenced artifacts) from one content tree to another, replacing
// the target folders exactly (files deleted in the source are deleted).
// It is the transport for a layout with a branch or repository per tier.
func CopyArtifacts(ctx context.Context, from, to string, ids []string, withDeps, dryRun bool) (*CopyResult, error) {
	set, err := ResolveTransport(ctx, from, ids, withDeps)
	if err != nil {
		return nil, err
	}
	res := &CopyResult{DryRun: dryRun, Artifacts: []CopiedItem{}}
	for _, a := range set.Artifacts {
		src := filepath.Join(from, filepath.FromSlash(a.Path))
		dst := filepath.Join(to, filepath.FromSlash(a.Path))
		it := CopiedItem{ID: a.ID, Path: a.Path, Action: "created"}
		if _, err := os.Stat(dst); err == nil {
			hs, err := file.TreeHash(src)
			if err != nil {
				return nil, err
			}
			hd, err := file.TreeHash(dst)
			if err != nil {
				return nil, err
			}
			it.Action = map[bool]string{true: "unchanged", false: "updated"}[hs == hd]
		}
		if it.Action != "unchanged" && !dryRun {
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return nil, err
			}
			if err := file.ReplaceDir(src, dst); err != nil {
				return nil, err
			}
		}
		res.Artifacts = append(res.Artifacts, it)
	}
	return res, nil
}
