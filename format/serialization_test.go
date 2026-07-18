package format

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
		SchemaVersion: 1,
		Sources: []Source{
			{Name: "source", Title: "Source", Path: "sources/source.zip", Type: "curated"},
		},
		Resources: []Item{
			{Name: "item", Path: "item.png", ID: "assetsdb:source/item.png", Source: "source"},
		},
	}

	var encoded bytes.Buffer
	require.NoError(t, Encode(&encoded, want))

	got, err := Decode(&encoded)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestDecodeInvalidJSON(t *testing.T) {
	_, err := Decode(bytes.NewBufferString("{"))
	require.Error(t, err)
}
