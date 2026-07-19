package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCLIUsageIdentifiesBuildCommand(t *testing.T) {
	var output bytes.Buffer
	printUsage(&output)

	require.Contains(t, output.String(), "assetsdb build")
	require.Contains(t, output.String(), "--manifest")
	require.Contains(t, output.String(), "--out")
}
