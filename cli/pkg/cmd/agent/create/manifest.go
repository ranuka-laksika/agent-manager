// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
)

const (
	manifestAPIVersion = "agent-manager.wso2.com/v1alpha1"
	manifestKind       = "Agent"
)

// Manifest is the versioned wrapper around the create-agent request body.
// Spec stays raw so the envelope can be validated before the payload.
type Manifest struct {
	APIVersion string          `json:"apiVersion" yaml:"apiVersion"`
	Kind       string          `json:"kind"       yaml:"kind"`
	Spec       json.RawMessage `json:"spec"       yaml:"spec"`
}

func loadManifest(path string) (amsvc.CreateAgentRequest, error) {
	var req amsvc.CreateAgentRequest

	data, err := os.ReadFile(path)
	if err != nil {
		return req, fmt.Errorf("read manifest: %w", err)
	}

	var m Manifest
	if err := DecodeStrict(data, &m); err != nil {
		return req, fmt.Errorf("parse manifest: %w", err)
	}
	if m.APIVersion != manifestAPIVersion {
		return req, fmt.Errorf("manifest apiVersion must be %q, got %q", manifestAPIVersion, m.APIVersion)
	}
	if m.Kind != manifestKind {
		return req, fmt.Errorf("manifest kind must be %q, got %q", manifestKind, m.Kind)
	}
	if len(m.Spec) == 0 {
		return req, fmt.Errorf("manifest spec is required")
	}

	dec := json.NewDecoder(bytes.NewReader(m.Spec))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, fmt.Errorf("parse manifest spec: %w", err)
	}

	if err := strictBuildUnion(req.Build); err != nil {
		return req, fmt.Errorf("parse manifest spec: %w", err)
	}
	return req, nil
}

// strictBuildUnion extends unknown-field rejection into the build block:
// Build.UnmarshalJSON stores raw bytes without field checking, so the
// top-level strict decode cannot see inside the union. An unknown
// discriminator is not an error here — validateRequest reports it so it
// lands in the aggregated violation list.
func strictBuildUnion(b *amsvc.Build) error {
	if b == nil {
		return nil
	}
	disc, err := b.Discriminator()
	if err != nil {
		return nil
	}
	raw, err := b.MarshalJSON()
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	switch disc {
	case buildTypeBuildpack:
		var v amsvc.BuildpackBuild
		return dec.Decode(&v)
	case buildTypeDocker:
		var v amsvc.DockerBuild
		return dec.Decode(&v)
	}
	return nil
}
