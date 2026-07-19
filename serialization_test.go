package assetsdb

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	want := &DataPackage{
		Name:          "assetsdb-test",
		Title:         "Test Package",
		Version:       "1.0.0",
		Created:       "2026-07-18T00:00:00Z",
		Licenses:      []License{{Name: "CC0-1.0"}},
		SchemaVersion: 2,
		Sources: []Source{
			{Name: "source", Title: "Source", Path: "sources/source.zip", Type: "curated", ExtractionPolicy: ExtractionSemanticFiles},
		},
		Resources: []Item{
			{Name: "item", Path: "item.png", ID: "assetsdb:source/item.png", Source: "source", ExtractionPolicy: ExtractionSemanticFiles, NamingEvidence: NamingEvidence{Kind: "file-basename", Value: "item"}},
		},
	}

	var encoded bytes.Buffer
	require.NoError(t, Encode(&encoded, want))

	got, err := Decode(&encoded)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestValidateDataPackageSchema2ExtractionMetadata(t *testing.T) {
	policy := ExtractionSemanticFiles
	valid := &DataPackage{
		SchemaVersion: 2,
		Sources:       []Source{{Name: "pack", ExtractionPolicy: policy}},
		Resources: []Item{{ID: "assetsdb:pack/door.png", Source: "pack", ExtractionPolicy: policy,
			NamingEvidence: NamingEvidence{Kind: "file-basename", Value: "door"}}},
	}
	require.NoError(t, validateDataPackage(valid))

	missingPolicy := *valid
	missingPolicy.Sources = []Source{{Name: "pack"}}
	require.Error(t, validateDataPackage(&missingPolicy))

	missingEvidence := *valid
	missingEvidence.Resources = []Item{{ID: "assetsdb:pack/door.png", Source: "pack", ExtractionPolicy: policy}}
	require.Error(t, validateDataPackage(&missingEvidence))

	require.Error(t, validateDataPackage(&DataPackage{SchemaVersion: 3}))
}

func TestDecodeInvalidJSON(t *testing.T) {
	_, err := Decode(bytes.NewBufferString("{"))
	require.Error(t, err)
}
