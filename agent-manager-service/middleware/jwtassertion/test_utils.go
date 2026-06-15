// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package jwtassertion

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// NewMockMiddleware creates a mock JWT middleware for testing.
// Automatically extracts org from the request path if it contains /orgs/{orgName}/
func NewMockMiddleware(t *testing.T) Middleware {
	t.Helper()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Extract org from path if present
			orgName := extractOrgFromPath(r.URL.Path)
			if orgName == "" {
				orgName = "mock-org"
			}

			// Create token claims with extracted org
			tokenClaims := &TokenClaims{
				Scope:    "test-scopes",
				OuId:     "mock-org-id",
				OuHandle: orgName, // Use extracted org as handle
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				},
			}

			// Set the context values that the middleware expects
			ctx = context.WithValue(ctx, assertionTokenClaimsKey, tokenClaims)
			ctx = context.WithValue(ctx, jwtToken, "mock-jwt-token")
			ctx = context.WithValue(ctx, scopesKey, tokenClaims.Scope)

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// extractOrgFromPath extracts the org name from a path like /api/v1/orgs/{org}/...
func extractOrgFromPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "orgs" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
