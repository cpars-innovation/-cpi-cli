package cmd

import (
	"testing"

	"github.com/cpars-innovation/-cpi-cli/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPulledConfigureConfig(t *testing.T) {
	artifacts := []*api.ArtifactDetails{{Id: "FlowB"}, {Id: "FlowA"}}
	get := func(id, version string) (*api.ParametersData, error) {
		data := &api.ParametersData{}
		data.Root.Results = []*api.ParameterData{
			{ParameterKey: "Z_KEY", ParameterValue: id + "-z"},
			{ParameterKey: "A_KEY", ParameterValue: id + "-a"},
		}
		return data, nil
	}

	cfg, err := pulledConfigureConfig("Package", artifacts, get)
	require.NoError(t, err)
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	assert.Equal(t, `deploymentPrefix: ""
packages:
    - integrationSuiteId: Package
      artifacts:
        - artifactId: FlowA
          type: Integration
          parameters:
            - key: A_KEY
              value: FlowA-a
            - key: Z_KEY
              value: FlowA-z
        - artifactId: FlowB
          type: Integration
          parameters:
            - key: A_KEY
              value: FlowB-a
            - key: Z_KEY
              value: FlowB-z
`, string(data))
}

func TestFilterExistingPackages(t *testing.T) {
	existing, missing := filterExistingPackages(
		[]string{"PackageA", "Missing", "PackageB"},
		[]string{"PackageA", "PackageB"},
	)

	assert.Equal(t, []string{"PackageA", "PackageB"}, existing)
	assert.Equal(t, []string{"Missing"}, missing)
}
