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

package tests

import (
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
)

// SetupRequestContext injects org context into a request for unit testing controllers.
// This is useful for unit tests that call controllers directly without going through middleware.
func SetupRequestContext(r *http.Request, orgName string) *http.Request {
	ctx := r.Context()
	ctx = middleware.WithResolvedOrg(ctx, middleware.ResolvedOrg{
		OuHandle: orgName,
		OUID:     "test-org-id",
	})
	return r.WithContext(ctx)
}

// ContextInjectorMiddleware wraps an http.ResponseWriter and http.Request to
// automatically inject org context from the path for unit tests that don't go through middleware.
type ContextInjectorMiddleware struct {
	handler http.Handler
}

// NewContextInjectorMiddleware creates middleware that injects context based on the request path.
func NewContextInjectorMiddleware(handler http.Handler) http.Handler {
	return &ContextInjectorMiddleware{handler: handler}
}

// ServeHTTP implements http.Handler by injecting org context from the URL path.
func (m *ContextInjectorMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract org from path (e.g., /api/v1/orgs/{org}/...)
	orgName := extractOrgFromPath(r.URL.Path)
	if orgName != "" {
		r = SetupRequestContext(r, orgName)
	}
	m.handler.ServeHTTP(w, r)
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
