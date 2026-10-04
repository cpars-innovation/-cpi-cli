//go:build integration

// Tenant integration tests: these create, deploy and delete content on a real
// SAP Integration Suite tenant. Run explicitly with `go test -tags integration`.

package cpi

import (
	"fmt"
	"os"
	"testing"

	"github.com/cpars-innovation/cpicli/internal/file"
	"github.com/cpars-innovation/cpicli/internal/logger"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type APIProductSuite struct {
	suite.Suite
	serviceDetails *ServiceDetails
	exe            *httpclnt.HTTPExecuter
}

func TestAPIProductOauth(t *testing.T) {
	suite.Run(t, &APIProductSuite{
		serviceDetails: &ServiceDetails{
			Host:              os.Getenv("FLASHPIPE_APIPORTAL_HOST"),
			OauthHost:         os.Getenv("FLASHPIPE_OAUTH_HOST"),
			OauthPath:         os.Getenv("FLASHPIPE_OAUTH_PATH"),
			OauthClientId:     os.Getenv("FLASHPIPE_APIPORTAL_OAUTH_CLIENTID"),
			OauthClientSecret: os.Getenv("FLASHPIPE_APIPORTAL_OAUTH_CLIENTSECRET"),
		},
	})
}

func (suite *APIProductSuite) SetupSuite() {
	println("========== Setting up suite - start ==========")
	suite.exe = InitHTTPExecuter(suite.serviceDetails)

	// Setup viper in case debug logs are required
	viper.SetEnvPrefix("FLASHPIPE")
	viper.AutomaticEnv()
	logger.InitConsoleLogger(viper.GetBool("debug"))

	setupAPIProxy(suite.T(), "Northwind_V4", suite.exe)

	println("========== Setting up suite - end ==========")
}

func (suite *APIProductSuite) SetupTest() {
	println("---------- Setting up test - start ----------")
	println("---------- Setting up test - end ----------")
}

func (suite *APIProductSuite) TearDownTest() {
	println("---------- Tearing down test - start ----------")
	println("---------- Tearing down test - end ----------")
}

func (suite *APIProductSuite) TearDownSuite() {
	println("========== Tearing down suite - start ==========")

	tearDownAPIProduct(suite.T(), "Northwind", suite.exe)
	tearDownAPIProxy(suite.T(), "Northwind_V4", suite.exe)
	err := os.RemoveAll("../../output/apiproduct")
	if err != nil {
		suite.T().Logf("WARNING - Directory removal failed with error - %v", err)
	}
	println("========== Tearing down suite - end ==========")
}

func (suite *APIProductSuite) TestAPIProduct_1_Upload() {
	a := NewAPIProduct(suite.exe)

	err := a.Upload("../../test/testdata/apiproduct/Northwind.json", "../../output/apiproduct/work/upload")
	if err != nil {
		suite.T().Fatalf("Upload APIProduct failed with error - %v", err)
	}
	productExists, err := a.Exists("Northwind")
	if err != nil {
		suite.T().Fatalf("Get APIProduct failed with error %v", err)
	}
	assert.True(suite.T(), productExists, "APIProduct was not uploaded")

	proxies, err := a.List()
	if err != nil {
		suite.T().Fatalf("List APIProducts failed with error - %v", err)
	}
	assert.GreaterOrEqual(suite.T(), len(proxies), 1, "Expected number of APIProducts >= 1")
}

func (suite *APIProductSuite) TestAPIProduct_2_Download() {
	a := NewAPIProduct(suite.exe)

	err := a.Download("Northwind", "../../output/apiproduct/work/download")
	if err != nil {
		suite.T().Fatalf("Download APIProduct failed with error - %v", err)
	}

	assert.True(suite.T(), file.Exists("../../output/apiproduct/work/download/Northwind.json"), "APIProduct was not downloaded")
}

func setupAPIProxy(t *testing.T, id string, exe *httpclnt.HTTPExecuter) {
	a := NewAPIProxy(exe)

	proxyExists, err := a.Exists(id)
	if err != nil {
		t.Logf("WARNING - Exists failed with error - %v", err)
	}
	if !proxyExists {
		err := a.Upload(fmt.Sprintf("../../test/testdata/apiproxy/%v", id), "../../output/apiproduct/work/upload")
		if err != nil {
			t.Fatalf("Upload APIProxy failed with error - %v", err)
		}
	}
}

func tearDownAPIProduct(t *testing.T, id string, exe *httpclnt.HTTPExecuter) {
	a := NewAPIProduct(exe)

	productExists, err := a.Exists(id)
	if err != nil {
		t.Logf("WARNING - Exists failed with error - %v", err)
	}
	if productExists {
		err = a.Delete(id)
		if err != nil {
			t.Logf("WARNING - Delete failed with error - %v", err)
		}
	}
}
