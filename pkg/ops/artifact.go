package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/internal/sync"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
)

// UploadRequest describes a local artifact directory to upload.
type UploadRequest struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"` // defaults to ID
	Type      string `json:"type"`           // Integration, MessageMapping, ScriptCollection, ValueMapping
	PackageID string `json:"packageId"`
	Dir       string `json:"dir"` // contains META-INF/MANIFEST.MF and src/main/resources
}

// UploadResult is the outcome of UploadArtifact.
type UploadResult struct {
	ID        string `json:"id"`
	PackageID string `json:"packageId"`
	sync.UploadOutcome
}

// UploadArtifact creates the designtime artifact or updates it if the local
// content differs. It does not deploy.
func UploadArtifact(exe *httpclnt.HTTPExecuter, req UploadRequest) (*UploadResult, error) {
	if !cpi.IsValidArtifactType(req.Type) {
		return nil, output.Usagef("invalid artifact type %q (valid types: %s)", req.Type, strings.Join(cpi.ArtifactTypes, ", "))
	}
	if req.ID == "" || req.PackageID == "" || req.Dir == "" {
		return nil, output.Usagef("artifact id, package id and directory are required")
	}
	if info, err := os.Stat(req.Dir); err != nil || !info.IsDir() {
		return nil, output.Usagef("artifact directory %q does not exist", req.Dir)
	}
	if req.Name == "" {
		// as the CLI: the display name from the manifest, else the ID
		if mf, err := os.ReadFile(filepath.Join(req.Dir, "META-INF", "MANIFEST.MF")); err == nil {
			req.Name = parseManifest(mf)["Bundle-Name"]
		}
	}
	if req.Name == "" {
		req.Name = req.ID
	}
	workDir, err := os.MkdirTemp("", "cpicli-upload-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	outcome, err := sync.New(exe).UploadArtifact(req.ID, req.Name, req.Type, req.PackageID, req.Dir, workDir, "", nil)
	if err != nil {
		return nil, err
	}
	return &UploadResult{ID: req.ID, PackageID: req.PackageID, UploadOutcome: outcome}, nil
}
