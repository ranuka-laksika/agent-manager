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

package controllers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
)

// ouIDStub is a minimal IdentityClient stub that only overrides GetOUIDByHandle.
// All other interface methods are provided by the embedded nil interface and
// must not be called during these tests.
type ouIDStub struct {
	thundersvc.IdentityClient
	mu      sync.Mutex
	results []struct {
		id  string
		err error
	}
	callCount int
}

func (s *ouIDStub) GetOUIDByHandle(_ context.Context, _ string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.callCount
	s.callCount++
	if i >= len(s.results) {
		return "", errors.New("unexpected GetOUIDByHandle call")
	}
	r := s.results[i]
	return r.id, r.err
}

func newRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/", nil)
}

func TestResolveOuID_FailureThenSuccess(t *testing.T) {
	lookupErr := errors.New("thunder unavailable")
	stub := &ouIDStub{
		results: []struct {
			id  string
			err error
		}{
			{id: "", err: lookupErr},
			{id: "root-ou-123", err: nil},
		},
	}
	ctrl := &identityController{client: stub}

	// First call: Thunder fails — error returned, nothing cached.
	_, err := ctrl.resolveOuID(newRequest(), "default")
	if err == nil {
		t.Fatal("expected error on first call, got nil")
	}
	if ctrl.ouIDByOrg["default"] != "" {
		t.Fatalf("ouIDByOrg should not be cached after failure, got %q", ctrl.ouIDByOrg["default"])
	}

	// Second call: Thunder succeeds — ID cached, no error.
	id, err := ctrl.resolveOuID(newRequest(), "default")
	if err != nil {
		t.Fatalf("expected no error on second call, got %v", err)
	}
	if id != "root-ou-123" {
		t.Fatalf("expected %q, got %q", "root-ou-123", id)
	}
	if ctrl.ouIDByOrg["default"] != "root-ou-123" {
		t.Fatalf("ouIDByOrg should be cached after success, got %q", ctrl.ouIDByOrg["default"])
	}
	if stub.callCount != 2 {
		t.Fatalf("expected 2 Thunder calls, got %d", stub.callCount)
	}
}

func TestResolveOuID_CachesSuccessAfterFirstCall(t *testing.T) {
	stub := &ouIDStub{
		results: []struct {
			id  string
			err error
		}{
			{id: "root-ou-abc", err: nil},
		},
	}
	ctrl := &identityController{client: stub}

	for i := range 5 {
		id, err := ctrl.resolveOuID(newRequest(), "default")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if id != "root-ou-abc" {
			t.Fatalf("call %d: expected %q, got %q", i, "root-ou-abc", id)
		}
	}

	// Thunder must have been called exactly once — the rest served from cache.
	if stub.callCount != 1 {
		t.Fatalf("expected exactly 1 Thunder call, got %d", stub.callCount)
	}
}

func TestResolveOuID_ConcurrentFailureThenSuccess(t *testing.T) {
	lookupErr := errors.New("transient error")
	// Enough results for multiple concurrent first-round failures, then one success.
	var results []struct {
		id  string
		err error
	}
	for range 20 {
		results = append(results, struct {
			id  string
			err error
		}{id: "", err: lookupErr})
	}
	results = append(results, struct {
		id  string
		err error
	}{id: "root-ou-concurrent", err: nil})

	stub := &ouIDStub{results: results}
	ctrl := &identityController{client: stub}

	// Drive concurrent failures until one goroutine eventually succeeds.
	var wg sync.WaitGroup
	var successCount, failCount int
	var mu sync.Mutex

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := ctrl.resolveOuID(newRequest(), "default")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failCount++
			} else if id == "root-ou-concurrent" {
				successCount++
			}
		}()
	}
	wg.Wait()

	// After failures, explicitly call until success (bounded to avoid CI hangs).
	const maxRetries = 30
	for i := range maxRetries {
		if ctrl.ouIDByOrg["default"] != "" {
			break
		}
		if _, err := ctrl.resolveOuID(newRequest(), "default"); err != nil && i == maxRetries-1 {
			t.Fatalf("ouIDByOrg not set after %d retries; last error: %v", maxRetries, err)
		}
	}
	if ctrl.ouIDByOrg["default"] == "" {
		t.Fatalf("ouIDByOrg not set after %d retries", maxRetries)
	}

	// After a success is cached, all further calls return the cached value.
	for i := range 5 {
		id, err := ctrl.resolveOuID(newRequest(), "default")
		if err != nil {
			t.Fatalf("post-cache call %d: unexpected error: %v", i, err)
		}
		if id != "root-ou-concurrent" {
			t.Fatalf("post-cache call %d: expected cached ID, got %q", i, id)
		}
	}
	if stub.callCount > len(results) {
		t.Fatalf("Thunder called %d times, only %d results provided", stub.callCount, len(results))
	}
}
