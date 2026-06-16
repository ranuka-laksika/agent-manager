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

package llmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

func sampleProviderResponse() amsvc.LLMProviderResponse {
	return amsvc.LLMProviderResponse{
		Uuid:     "11111111-1111-1111-1111-111111111111",
		Id:       "openai",
		Name:     "OpenAI",
		Version:  "v1",
		Context:  "/",
		Template: "openai",
		Status:   amsvc.LLMProviderResponseStatus("pending"),
	}
}

// authValue digs out upstream.main.auth.value from a captured create body.
func authValue(t *testing.T, raw []byte) (string, bool) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal body: %v\nbody=%s", err, raw)
	}
	up, ok := m["upstream"].(map[string]any)
	if !ok {
		return "", false
	}
	main, ok := up["main"].(map[string]any)
	if !ok {
		return "", false
	}
	auth, ok := main["auth"].(map[string]any)
	if !ok {
		return "", false
	}
	v, ok := auth["value"].(string)
	return v, ok
}

func TestCreate_SuccessWithAPIKey(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusCreated, sampleProviderResponse())
	defer closeFn()

	err := runCreate(context.Background(), &CreateOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: "v1", Context: "/",
		Template: "openai", AuthType: "api-key", APIKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.method != "POST" {
		t.Errorf("method = %q, want POST", captured.method)
	}
	if captured.path != "/orgs/acme/llm-providers" {
		t.Errorf("path = %q, want /orgs/acme/llm-providers", captured.path)
	}
	if v, ok := authValue(t, captured.body); !ok || v != "sk-test" {
		t.Errorf("upstream auth value = %q (ok=%v), want sk-test", v, ok)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	if data["id"] != "openai" {
		t.Errorf("id = %v, want openai", data["id"])
	}
}

func TestCreate_MinimalDefersToTemplate(t *testing.T) {
	io, _, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusCreated, sampleProviderResponse())
	defer closeFn()

	// No key, no upstream overrides, no explicit auth flags: the request must
	// not carry an upstream auth block — the template supplies it.
	err := runCreate(context.Background(), &CreateOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: "v1", Context: "/",
		Template: "openai", AuthType: "api-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := authValue(t, captured.body); ok {
		t.Errorf("expected no upstream auth block, body=%s", captured.body)
	}
}

func TestCreate_APIKeyStdin(t *testing.T) {
	io, _, _ := newTestIOWithStdin(true, "sk-stdin\n")
	clientFn, captured, closeFn := newTestClient(t, http.StatusCreated, sampleProviderResponse())
	defer closeFn()

	err := runCreate(context.Background(), &CreateOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: "v1", Context: "/",
		Template: "openai", AuthType: "api-key", APIKeyStdin: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := authValue(t, captured.body); !ok || v != "sk-stdin" {
		t.Errorf("upstream auth value = %q (ok=%v), want sk-stdin", v, ok)
	}
}

func TestCreate_EmptyStdinKey(t *testing.T) {
	io, out, _ := newTestIOWithStdin(true, "   \n")
	err := runCreate(context.Background(), &CreateOptions{
		IO: io, Client: unreachableClient, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: "v1", Context: "/",
		Template: "openai", AuthType: "api-key", APIKeyStdin: true,
	})
	if err == nil {
		t.Fatal("expected error for empty stdin key")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.InvalidFlag {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.InvalidFlag)
	}
}

func TestCreate_Conflict(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, _, closeFn := newTestClient(t, http.StatusConflict, amsvc.ErrorResponse{
		Code:    "LLM_PROVIDER_ALREADY_EXISTS",
		Message: "provider 'openai' already exists",
	})
	defer closeFn()

	err := runCreate(context.Background(), &CreateOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: "v1", Context: "/",
		Template: "openai", AuthType: "api-key", APIKey: "sk-test",
	})
	if err == nil {
		t.Fatal("expected error for 409")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != "LLM_PROVIDER_ALREADY_EXISTS" {
		t.Errorf("code = %v, want LLM_PROVIDER_ALREADY_EXISTS", errBody["code"])
	}
}

// --- flag-parsing / validation via the cobra tree ---

func runTreeExpectViolation(t *testing.T, args []string, wants ...string) {
	t.Helper()
	ios, out, _ := newTestIO(true)
	cmd := testLLMProviderCmd(t, ios, nil)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected validation error")
	}
	env := decodeEnvelope(t, out.String())
	errMap := env["error"].(map[string]any)
	if errMap["code"] != clierr.InvalidFlag {
		t.Fatalf("code = %v, want %s", errMap["code"], clierr.InvalidFlag)
	}
	additional := errMap["additionalData"].(map[string]any)
	details, ok := additional["details"].([]any)
	if !ok {
		t.Fatalf("details type = %T", additional["details"])
	}
	for _, want := range wants {
		found := false
		for _, d := range details {
			if strings.Contains(d.(string), want) {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %q in details, got %v", want, details)
		}
	}
}

func TestCreate_MissingRequired(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create"},
		"id argument is required", "--display-name is required", "--template is required")
}

func TestCreate_IDWithSlash(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "bad/id", "--display-name", "X", "--template", "openai"},
		"id must not contain '/'")
}

func TestCreate_BadAuthType(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai", "--auth-type", "weird"},
		"--auth-type must be one of")
}

func TestCreate_MutuallyExclusiveKeys(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai", "--api-key", "k", "--api-key-stdin"},
		"--api-key and --api-key-stdin are mutually exclusive")
}

func TestCreate_KeyWithAuthNone(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai", "--auth-type", "none", "--api-key", "k"},
		"an API key cannot be used with --auth-type none")
}

func TestCreate_BadGateway(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai", "--gateways", "not-a-uuid"},
		"invalid gateway id")
}
