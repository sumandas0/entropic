package testutils

import (
	"testing"
)

func TestTypesenseContainerSetup(t *testing.T) {
	container, err := SetupTestTypesense()
	if err != nil {
		t.Fatalf("Failed to setup Typesense: %v", err)
	}
	defer container.Cleanup()
	
	t.Logf("Container URL: %s", container.URL)
	t.Logf("Container API Key: %s", container.APIKey)
	
	t.Log("Container setup successful")
}