package api

import (
	"fmt"
	"os"
	"testing"

	"github.com/cpars-innovation/cpicli/internal/file"
	"github.com/cpars-innovation/cpicli/internal/httpclnt"
	"github.com/stretchr/testify/assert"
)

func TestDesigntime_Compare(t *testing.T) {
	// List the artifacts that will be tested
	artifacts := map[string]string{
		"Integration":      "Integration_Test_IFlow",
		"MessageMapping":   "Integration_Test_Message_Mapping",
		"ScriptCollection": "Integration_Test_Script_Collection",
		"ValueMapping":     "Integration_Test_Value_Mapping",
	}
	exe := httpclnt.New("", "", "", "", "dummy", "dummy", "localhost", "http", 8081, true)

	for key, value := range artifacts {
		dt := NewDesigntimeArtifact(key, exe)
		compare(value, dt, t)
	}

	err := os.RemoveAll("../../output/download")
	if err != nil {
		t.Fatalf("Directory removal failed with error - %v", err)
	}
}
func compare(id string, dt DesigntimeArtifact, t *testing.T) {
	// Diff artifact content
	srcDir := fmt.Sprintf("../../test/testdata/artifacts/update/%v", id)
	tgtDir := fmt.Sprintf("../../test/testdata/artifacts/create/%v", id)
	dirDiffer, err := dt.CompareContent(srcDir, tgtDir, nil, "git")
	if err != nil {
		t.Fatalf("CompareContent failed with error - %v", err)
	}
	assert.True(t, dirDiffer, "Directory contents do not differ")

	// Copy to output folder
	destinationDir := fmt.Sprintf("../../output/download/%v", id)
	err = dt.CopyContent(srcDir, destinationDir)
	if err != nil {
		t.Fatalf("CopyContent failed with error - %v", err)
	}
	assert.True(t, file.Exists(destinationDir+"/META-INF/MANIFEST.MF"), "MANIFEST.MF missing in destination")
	switch dt.(type) {
	case *Integration, *MessageMapping, *ScriptCollection:
		assert.True(t, file.Exists(destinationDir+"/src/main/resources"), "/src/main/resources missing in destination")
	case *ValueMapping:
		assert.True(t, file.Exists(destinationDir+"/value_mapping.xml"), "value_mapping.xml missing in destination")
	}
}
