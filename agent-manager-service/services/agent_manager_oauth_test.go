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
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// policyByName returns the policy map with the given name from a policy slice, or nil.
func policyByName(policies []map[string]interface{}, name string) map[string]interface{} {
	for _, p := range policies {
		if p["name"] == name {
			return p
		}
	}
	return nil
}

func policyParams(p map[string]interface{}) map[string]interface{} {
	if p == nil {
		return nil
	}
	params, _ := p["params"].(map[string]interface{})
	return params
}

func TestValidateAuthExclusivity(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  bool
		oauth   bool
		wantErr bool
	}{
		{"none", false, false, false},
		{"api-key only", true, false, false},
		{"oauth only", false, true, false},
		{"both rejected", true, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAuthExclusivity(resolvedCORSConfig{
				EnableApiKeySecurity: tc.apiKey,
				EnableOAuthSecurity:  tc.oauth,
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, utils.ErrInvalidInput) {
					t.Fatalf("expected ErrInvalidInput, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestResolveAPIConfig_OAuthDefaults(t *testing.T) {
	// withDefaults, no request, no DB: API key on, OAuth off, header defaults set.
	cfg := resolveAPIConfig(nil, nil, nil, nil, nil, true)
	if !cfg.EnableApiKeySecurity {
		t.Errorf("expected API key security on by default")
	}
	if cfg.EnableOAuthSecurity {
		t.Errorf("expected OAuth security off by default")
	}
	if cfg.OAuthHeaderName != models.DefaultOAuthHeaderName {
		t.Errorf("expected header name %q, got %q", models.DefaultOAuthHeaderName, cfg.OAuthHeaderName)
	}
	if cfg.OAuthAuthHeaderPrefix != models.DefaultOAuthAuthHeaderPrefix {
		t.Errorf("expected header prefix %q, got %q", models.DefaultOAuthAuthHeaderPrefix, cfg.OAuthAuthHeaderPrefix)
	}
}

func TestResolveAPIConfig_OAuthFromRequest(t *testing.T) {
	cfg := resolveAPIConfig(nil, boolPtr(false), nil, boolPtr(true), &spec.OAuthConfig{
		Issuers:          []string{"CustomKeyManager"},
		Audiences:        []string{"aud1"},
		RequiredScopes:   []string{"read", "write"},
		RequiredClaims:   map[string]interface{}{"role": "admin"},
		HeaderName:       strPtr("X-Token"),
		AuthHeaderPrefix: strPtr("Token"),
	}, true)

	if cfg.EnableApiKeySecurity {
		t.Errorf("expected API key off")
	}
	if !cfg.EnableOAuthSecurity {
		t.Errorf("expected OAuth on")
	}
	if len(cfg.OAuthIssuers) != 1 || cfg.OAuthIssuers[0] != "CustomKeyManager" {
		t.Errorf("issuers not resolved from request: %v", cfg.OAuthIssuers)
	}
	if len(cfg.OAuthAudiences) != 1 || cfg.OAuthAudiences[0] != "aud1" {
		t.Errorf("audiences not resolved: %v", cfg.OAuthAudiences)
	}
	if len(cfg.OAuthRequiredScopes) != 2 {
		t.Errorf("scopes not resolved: %v", cfg.OAuthRequiredScopes)
	}
	if cfg.OAuthRequiredClaims["role"] != "admin" {
		t.Errorf("claims not resolved: %v", cfg.OAuthRequiredClaims)
	}
	if cfg.OAuthHeaderName != "X-Token" || cfg.OAuthAuthHeaderPrefix != "Token" {
		t.Errorf("header overrides not resolved: %q / %q", cfg.OAuthHeaderName, cfg.OAuthAuthHeaderPrefix)
	}
}

func TestResolveAPIConfig_OAuthFromDB(t *testing.T) {
	existing := &models.AgentConfig{
		EnableApiKeySecurity:  false,
		EnableOAuthSecurity:   true,
		OAuthIssuers:          []string{"DBKeyManager"},
		OAuthHeaderName:       "Authorization",
		OAuthAuthHeaderPrefix: "Bearer",
	}
	// No request fields → resolve from DB.
	cfg := resolveAPIConfig(existing, nil, nil, nil, nil, false)
	if !cfg.EnableOAuthSecurity {
		t.Errorf("expected OAuth on from DB")
	}
	if len(cfg.OAuthIssuers) != 1 || cfg.OAuthIssuers[0] != "DBKeyManager" {
		t.Errorf("issuers not resolved from DB: %v", cfg.OAuthIssuers)
	}

	// Request overrides DB.
	cfg = resolveAPIConfig(existing, boolPtr(true), nil, boolPtr(false), nil, false)
	if cfg.EnableOAuthSecurity {
		t.Errorf("expected OAuth disabled by request override")
	}
	if !cfg.EnableApiKeySecurity {
		t.Errorf("expected API key enabled by request override")
	}
}

func TestBuildPolicies_Modes(t *testing.T) {
	base := resolvedCORSConfig{
		CORSEnabled:           true,
		CORSAllowOrigins:      []string{"*"},
		CORSAllowMethods:      []string{"GET"},
		CORSAllowHeaders:      []string{"Authorization"},
		OAuthHeaderName:       models.DefaultOAuthHeaderName,
		OAuthAuthHeaderPrefix: models.DefaultOAuthAuthHeaderPrefix,
	}

	t.Run("no auth", func(t *testing.T) {
		p := buildPolicies(base)
		if policyByName(p, "cors") == nil {
			t.Errorf("expected cors policy")
		}
		if policyByName(p, "api-key-auth") != nil || policyByName(p, "jwt-auth") != nil {
			t.Errorf("expected no auth policies, got %v", p)
		}
	})

	t.Run("api key", func(t *testing.T) {
		cfg := base
		cfg.EnableApiKeySecurity = true
		p := buildPolicies(cfg)
		if policyByName(p, "api-key-auth") == nil {
			t.Errorf("expected api-key-auth policy")
		}
		if policyByName(p, "jwt-auth") != nil {
			t.Errorf("did not expect jwt-auth policy")
		}
	})

	t.Run("oauth with issuers and full params", func(t *testing.T) {
		cfg := base
		cfg.EnableOAuthSecurity = true
		cfg.OAuthIssuers = []string{"MyKeyManager"}
		cfg.OAuthAudiences = []string{"aud1"}
		cfg.OAuthRequiredScopes = []string{"read"}
		cfg.OAuthRequiredClaims = map[string]interface{}{"role": "admin"}
		p := buildPolicies(cfg)
		jwt := policyByName(p, "jwt-auth")
		if jwt == nil {
			t.Fatalf("expected jwt-auth policy")
		}
		if policyByName(p, "api-key-auth") != nil {
			t.Errorf("did not expect api-key-auth policy")
		}
		params := policyParams(jwt)
		issuers, _ := params["issuers"].([]string)
		if len(issuers) != 1 || issuers[0] != "MyKeyManager" {
			t.Errorf("expected issuers [MyKeyManager], got %v", params["issuers"])
		}
		if params["audiences"] == nil || params["requiredScopes"] == nil || params["requiredClaims"] == nil {
			t.Errorf("expected full param set, got %v", params)
		}
		if params["headerName"] != models.DefaultOAuthHeaderName || params["authHeaderPrefix"] != models.DefaultOAuthAuthHeaderPrefix {
			t.Errorf("expected default header params, got %v / %v", params["headerName"], params["authHeaderPrefix"])
		}
	})

	t.Run("oauth empty issuers falls back to configured default", func(t *testing.T) {
		cfg := base
		cfg.EnableOAuthSecurity = true
		cfg.OAuthIssuers = nil
		p := buildPolicies(cfg)
		jwt := policyByName(p, "jwt-auth")
		if jwt == nil {
			t.Fatalf("expected jwt-auth policy")
		}
		issuers, _ := policyParams(jwt)["issuers"].([]string)
		if len(issuers) != 1 || issuers[0] != "ThunderKeyManager" {
			t.Errorf("expected default issuer [ThunderKeyManager], got %v", issuers)
		}
	})
}
