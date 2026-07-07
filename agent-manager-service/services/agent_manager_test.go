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

package services

import (
	"errors"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/instrumentation"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestValidateInstrumentationVersion_UsesCatalog(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.11"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	if err := s.validateInstrumentationVersion("0.2.1"); err != nil {
		t.Errorf("0.2.1 should be valid: %v", err)
	}
	err := s.validateInstrumentationVersion("9.9.9")
	if err == nil {
		t.Fatal("9.9.9 should be invalid")
	}
	if !strings.Contains(err.Error(), "9.9.9") {
		t.Errorf("error %q should mention 9.9.9", err)
	}
}

func TestValidatePythonInstrumentationPair(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.10", "3.11"}, ImageRepository: "x"},
			{Version: "0.4.0", PythonVersions: []string{"3.12", "3.13"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	if err := s.validatePythonInstrumentationPair("3.11", "0.2.1"); err != nil {
		t.Errorf("3.11 + 0.2.1 should be valid: %v", err)
	}
	err := s.validatePythonInstrumentationPair("3.13", "0.2.1")
	if err == nil {
		t.Fatal("3.13 + 0.2.1 should be invalid")
	}
	if !strings.Contains(err.Error(), "3.13") || !strings.Contains(err.Error(), "0.2.1") {
		t.Errorf("error %q should mention both python and instrumentation versions", err)
	}
}

func TestValidateEffectivePair_FallsBackToDefault(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.10", "3.11"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	// nil requested version means "use platform default", which is 0.2.1.
	if err := s.validateEffectivePythonInstrumentationPair("3.11", nil); err != nil {
		t.Errorf("3.11 + default(0.2.1) should be valid: %v", err)
	}
	err := s.validateEffectivePythonInstrumentationPair("3.13", nil)
	if err == nil {
		t.Fatal("3.13 + default(0.2.1) should be invalid")
	}
	if !strings.Contains(err.Error(), "3.13") || !strings.Contains(err.Error(), "0.2.1") {
		t.Errorf("error %q should name the resolved default version, not just nil", err)
	}
}

func TestBuildpackPythonVersion_Normalises(t *testing.T) {
	mk := func(lang string, version *string) *spec.Build {
		b := spec.BuildpackBuildAsBuild(&spec.BuildpackBuild{
			Buildpack: spec.BuildpackConfig{
				Language:        lang,
				LanguageVersion: version,
			},
		})
		return &b
	}
	strPtr := func(s string) *string { return &s }

	cases := []struct {
		name string
		in   *spec.Build
		want string
	}{
		{"bare minor", mk("python", strPtr("3.11")), "3.11"},
		{"with patch", mk("python", strPtr("3.11.4")), "3.11"},
		{"with x", mk("python", strPtr("3.11.x")), "3.11"},
		{"leading whitespace", mk("python", strPtr("  3.11  ")), "3.11"},
		{"whitespace only", mk("python", strPtr("   ")), ""},
		{"empty", mk("python", strPtr("")), ""},
		{"capital P language", mk("Python", strPtr("3.11")), ""},
		{"non python language", mk("nodejs", strPtr("20")), ""},
		{"single component", mk("python", strPtr("3")), ""},
		{"nil version", mk("python", nil), ""},
		{"nil build", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildpackPythonVersion(tc.in)
			if got != tc.want {
				t.Errorf("buildpackPythonVersion = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateEffectivePair_NoPythonIsNoOp(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.11"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	// Empty python means the agent isn't a python-buildpack build.
	if err := s.validateEffectivePythonInstrumentationPair("", nil); err != nil {
		t.Errorf("empty python should be a no-op: %v", err)
	}
}

func TestNormalizePythonMinor(t *testing.T) {
	cases := map[string]string{
		"3.11":     "3.11",
		"3.11.4":   "3.11",
		"3.11.x":   "3.11",
		"  3.11  ": "3.11",
		"3":        "",
		"":         "",
		"   ":      "",
	}
	for in, want := range cases {
		if got := normalizePythonMinor(in); got != want {
			t.Errorf("normalizePythonMinor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveInstrumentationImageOverride(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.10", "3.11"}, ImageRepository: "x"},
			{Version: "0.4.0", PythonVersions: []string{"3.12", "3.13"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	strPtr := func(s string) *string { return &s }
	s := &agentManagerService{logger: discardLogger()}

	t.Run("non-python echoes existing pin, no image", func(t *testing.T) {
		version, image, err := s.resolveInstrumentationImageOverride(false, "3.11", strPtr("0.4.0"), strPtr("0.2.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want existing 0.2.1", version)
		}
		if image != "" {
			t.Errorf("image = %q, want empty for non-python", image)
		}
	})

	t.Run("request override validates and wins", func(t *testing.T) {
		version, image, err := s.resolveInstrumentationImageOverride(true, "3.11", strPtr("0.2.1"), strPtr("0.4.0"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want requested 0.2.1", version)
		}
		if !strings.HasSuffix(image, "0.2.1-python3.11") {
			t.Errorf("image = %q, want suffix 0.2.1-python3.11", image)
		}
	})

	t.Run("unknown request version is rejected", func(t *testing.T) {
		_, _, err := s.resolveInstrumentationImageOverride(true, "3.11", strPtr("9.9.9"), nil)
		if !errors.Is(err, utils.ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("python-incompatible request version is rejected", func(t *testing.T) {
		// 0.4.0 supports 3.12/3.13, not 3.11.
		_, _, err := s.resolveInstrumentationImageOverride(true, "3.11", strPtr("0.4.0"), nil)
		if !errors.Is(err, utils.ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("no request preserves existing pin as image", func(t *testing.T) {
		version, image, err := s.resolveInstrumentationImageOverride(true, "3.11", nil, strPtr("0.2.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want preserved 0.2.1", version)
		}
		if !strings.HasSuffix(image, "0.2.1-python3.11") {
			t.Errorf("image = %q, want suffix 0.2.1-python3.11", image)
		}
	})

	t.Run("no request and no existing pin yields no override", func(t *testing.T) {
		version, image, err := s.resolveInstrumentationImageOverride(true, "3.11", nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version != nil {
			t.Errorf("version = %v, want nil", version)
		}
		if image != "" {
			t.Errorf("image = %q, want empty", image)
		}
	})

	t.Run("existing pin incompatible with current python keeps version but skips image", func(t *testing.T) {
		// Pin 0.2.1 supports 3.10/3.11; the agent's Python is now 3.13. Building
		// the image would yield a nonexistent 0.2.1-python3.13 tag, so the
		// override is skipped (empty image) while the DB version is preserved.
		version, image, err := s.resolveInstrumentationImageOverride(true, "3.13", nil, strPtr("0.2.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want preserved 0.2.1", version)
		}
		if image != "" {
			t.Errorf("image = %q, want empty (incompatible pin, component default kept)", image)
		}
	})

	t.Run("existing pin with unparseable python keeps component default", func(t *testing.T) {
		// No request override + bad language version: don't fail the redeploy,
		// just skip the per-env image override.
		version, image, err := s.resolveInstrumentationImageOverride(true, "notaversion", nil, strPtr("0.2.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want preserved 0.2.1", version)
		}
		if image != "" {
			t.Errorf("image = %q, want empty (component default kept)", image)
		}
	})
}
