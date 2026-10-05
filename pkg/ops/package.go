package ops

import (
	"regexp"

	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
)

// PackageRequest describes an integration package to create.
type PackageRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ShortText   string `json:"shortText,omitempty"`
}

// PackageResult is the outcome of CreatePackage.
type PackageResult struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Action string `json:"action"` // CREATED or EXISTS
}

var packageIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)

// CreatePackage creates an integration package unless one with the ID exists
// (action EXISTS; the existing package is not changed).
func CreatePackage(exe *httpclnt.HTTPExecuter, req PackageRequest) (*PackageResult, error) {
	if !packageIDPattern.MatchString(req.ID) {
		return nil, output.Usagef("invalid package ID %q: letters, digits, '_' and '.' only", req.ID)
	}
	if req.Name == "" {
		req.Name = req.ID
	}
	ip := cpi.NewIntegrationPackage(exe)
	existing, _, exists, err := ip.Get(req.ID)
	if err != nil {
		return nil, err
	}
	if exists {
		return &PackageResult{ID: req.ID, Name: existing.Root.Name, Action: "EXISTS"}, nil
	}
	shortText := req.ShortText
	if shortText == "" {
		shortText = req.Name
	}
	data := &cpi.PackageSingleData{Root: cpi.PackageFields{Id: req.ID, Name: req.Name, Description: req.Description, ShortText: shortText}}
	if err := ip.Create(data); err != nil {
		return nil, err
	}
	return &PackageResult{ID: req.ID, Name: req.Name, Action: "CREATED"}, nil
}
