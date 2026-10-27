package assetsdb

import (
	"encoding/json"
	"fmt"
	"io"
)

// Decode reads a DataPackage from its JSON representation.
func Decode(r io.Reader) (*DataPackage, error) {
	var dataPackage DataPackage
	if err := json.NewDecoder(r).Decode(&dataPackage); err != nil {
		return nil, fmt.Errorf("assetsdb format: decode data package: %w", err)
	}

	return &dataPackage, nil
}

// Encode writes a DataPackage as indented JSON.
func Encode(w io.Writer, dataPackage *DataPackage) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(dataPackage); err != nil {
		return fmt.Errorf("assetsdb format: encode data package: %w", err)
	}

	return nil
}
