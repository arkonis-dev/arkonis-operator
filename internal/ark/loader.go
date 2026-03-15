// Package ark provides shared utilities for the ark CLI.
package ark

import (
	"bytes"
	"fmt"
	"os"

	sigsyaml "sigs.k8s.io/yaml"

	arkonisv1alpha1 "github.com/arkonis-dev/ark-operator/api/v1alpha1"
)

// LoadFile reads a multi-document YAML file and returns all ArkFlow and ArkAgent
// resources it contains. Unknown kinds are silently skipped — this matches the
// behaviour of kubectl apply -f, where non-matching resources are simply ignored.
func LoadFile(path string) ([]*arkonisv1alpha1.ArkFlow, map[string]*arkonisv1alpha1.ArkAgent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return ParseDocs(data)
}

// ParseDocs parses raw multi-document YAML bytes into ArkFlow and ArkAgent objects.
// Exported so callers can parse in-memory YAML without a file (e.g. tests).
func ParseDocs(data []byte) ([]*arkonisv1alpha1.ArkFlow, map[string]*arkonisv1alpha1.ArkAgent, error) {
	var flows []*arkonisv1alpha1.ArkFlow
	agents := make(map[string]*arkonisv1alpha1.ArkAgent)

	for i, part := range splitDocs(data) {
		var meta struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		}
		if err := sigsyaml.Unmarshal(part, &meta); err != nil || meta.Kind == "" {
			continue
		}

		switch meta.Kind {
		case "ArkFlow":
			var f arkonisv1alpha1.ArkFlow
			if err := sigsyaml.Unmarshal(part, &f); err != nil {
				return nil, nil, fmt.Errorf("document %d (ArkFlow): %w", i, err)
			}
			flows = append(flows, &f)

		case "ArkAgent":
			var a arkonisv1alpha1.ArkAgent
			if err := sigsyaml.Unmarshal(part, &a); err != nil {
				return nil, nil, fmt.Errorf("document %d (ArkAgent): %w", i, err)
			}
			agents[a.Name] = &a
		}
	}

	return flows, agents, nil
}

// splitDocs splits a YAML byte slice on --- document separators.
func splitDocs(data []byte) [][]byte {
	var docs [][]byte
	for part := range bytes.SplitSeq(data, []byte("\n---")) {
		part = bytes.TrimPrefix(bytes.TrimSpace(part), []byte("---"))
		part = bytes.TrimSpace(part)
		if len(part) > 0 {
			docs = append(docs, part)
		}
	}
	return docs
}
