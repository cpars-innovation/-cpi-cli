//go:build integration

// Tenant integration tests: these create, deploy and delete content on a real
// SAP Integration Suite tenant. Run explicitly with `go test -tags integration`.

package cpi

import (
	"fmt"
	"github.com/cpars-innovation/cpicli/internal/file"
	"github.com/cpars-innovation/cpicli/internal/logger"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"os"
	"strings"
	"testing"
)

type DesigntimeSuite struct {
	suite.Suite
	serviceDetails *ServiceDetails
	exe            *httpclnt.HTTPExecuter
	artifacts      map[string]string
}

func TestDesigntimeBasicAuth(t *testing.T) {
	suite.Run(t, &DesigntimeSuite{
		serviceDetails: &ServiceDetails{
			Host:     os.Getenv("CPICTL_TMN_HOST"),
			Userid:   os.Getenv("CPICTL_TMN_USERID"),
			Password: os.Getenv("CPICTL_TMN_PASSWORD"),
		},
	})
}

func TestDesigntimeOauth(t *testing.T) {
	suite.Run(t, &DesigntimeSuite{
		serviceDetails: &ServiceDetails{
			Host:              os.Getenv("CPICTL_TMN_HOST"),
			OauthHost:         os.Getenv("CPICTL_OAUTH_HOST"),
			OauthPath:         os.Getenv("CPICTL_OAUTH_PATH"),
			OauthClientId:     os.Getenv("CPICTL_OAUTH_CLIENTID"),
			OauthClientSecret: os.Getenv("CPICTL_OAUTH_CLIENTSECRET"),
		},
	})
}

func (suite *DesigntimeSuite) SetupSuite() {
	println("========== Setting up suite - start ==========")
	suite.exe = InitHTTPExecuter(suite.serviceDetails)

	// List the artifacts that will be tested
	suite.artifacts = map[string]string{
		"Integration":      "Integration_Test_IFlow",
		"MessageMapping":   "Integration_Test_Message_Mapping",
		"ScriptCollection": "Integration_Test_Script_Collection",
		"ValueMapping":     "Integration_Test_Value_Mapping",
	}

	// Setup viper in case debug logs are required
	viper.SetEnvPrefix("CPICTL")
	viper.AutomaticEnv()
	logger.InitConsoleLogger(viper.GetBool("debug"))

	setupPackage(suite.T(), "CpictlIntegrationTest", suite.exe)
	println("========== Setting up suite - end ==========")
}

func (suite *DesigntimeSuite) SetupTest() {
	println("---------- Setting up test - start ----------")
	println("---------- Setting up test - end ----------")
}

func (suite *DesigntimeSuite) TearDownTest() {
	println("---------- Tearing down test - start ----------")
	println("---------- Tearing down test - end ----------")
}

func (suite *DesigntimeSuite) TearDownSuite() {
	println("========== Tearing down suite - start ==========")

	tearDownPackage(suite.T(), "CpictlIntegrationTest", suite.exe)

	// Remove all the runtime artifacts
	for _, value := range suite.artifacts {
		tearDownRuntime(suite.T(), value, suite.exe)
	}

	err := os.RemoveAll("../../output/download")
	if err != nil {
		suite.T().Logf("WARNING - Directory removal failed with error - %v", err)
	}
	println("========== Tearing down suite - end ==========")
}

func (suite *DesigntimeSuite) Test_CreateUpdateDeployDelete() {
	for artifactType, artifactId := range suite.artifacts {
		dt := NewDesigntimeArtifact(artifactType, suite.exe)
		createUpdateDeployDelete(artifactId, strings.ReplaceAll(artifactId, "_", " "), "CpictlIntegrationTest", dt, artifactType, suite.T())
	}
}

func createUpdateDeployDelete(id string, name string, packageId string, dt DesigntimeArtifact, artifactType string, t *testing.T) {
	// Create
	err := dt.Create(id, name, packageId, fmt.Sprintf("../../test/testdata/artifacts/create/%v", id))
	if err != nil {
		t.Fatalf("Create failed with error - %v", err)
	}
	// Check existence
	_, artifactDescription, artifactExists, err := dt.Get(id, "active")
	if err != nil {
		t.Fatalf("Exists failed with error - %v", err)
	}
	assert.Equal(t, fmt.Sprintf("%v Created", artifactType), artifactDescription, "Artifact has incorrect description")
	if assert.True(t, artifactExists, "Expected exists = true") {
		// Update
		err = dt.Update(id, name, packageId, fmt.Sprintf("../../test/testdata/artifacts/update/%v", id))
		if err != nil {
			t.Fatalf("Update failed with error - %v", err)
		}
		// Check version
		version, artifactDescriptionUpdated, _, err := dt.Get(id, "active")
		if err != nil {
			t.Fatalf("GetVersion failed with error - %v", err)
		}
		assert.Equal(t, fmt.Sprintf("%v Updated", artifactType), artifactDescriptionUpdated, "Artifact description not updated")
		if assert.Equal(t, "1.0.1", version, "Expected version = 1.0.1") {
			// Deploy
			err = dt.Deploy(id)
			if err != nil {
				t.Fatalf("Deploy failed with error - %v", err)
			}
			// Download
			targetFile := fmt.Sprintf("../../output/download/%v.zip", id)
			err = dt.Download(targetFile, id)
			if err != nil {
				t.Fatalf("Download failed with error - %v", err)
			}
			assert.Truef(t, file.Exists(targetFile), "Target file %v not found", targetFile)
			// Delete
			err = dt.Delete(id)
			if err != nil {
				t.Fatalf("Delete failed with error - %v", err)
			}
		}
	}
}

func setupArtifact(t *testing.T, artifactId string, packageId string, artifactDir string, artifactType string, exe *httpclnt.HTTPExecuter) {
	dt := NewDesigntimeArtifact(artifactType, exe)

	_, _, artifactExists, err := dt.Get(artifactId, "active")
	if err != nil {
		t.Logf("WARNING - Exists failed with error - %v", err)
	}
	if !artifactExists {
		err = dt.Create(artifactId, artifactId, packageId, artifactDir)
		if err != nil {
			t.Logf("WARNING - Create designtime artifact failed with error - %v", err)
		}
	}
}
