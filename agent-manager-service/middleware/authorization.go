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

package middleware

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// RequirePermission returns a middleware that checks the request token carries the
// required amp: scope. When RBAC_ENABLED=false the check is skipped entirely,
// allowing zero-downtime rollout.
func RequirePermission(perm rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !config.GetConfig().RBACEnabled {
				next(w, r)
				return
			}
			if !jwtassertion.HasAllScopes(r.Context(), []string{perm.Scope()}) {
				utils.WriteErrorResponse(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next(w, r)
		}
	}
}

// PermissionResolver resolves the required permission at request time.
// Returning an error causes the request to be rejected with 403.
type PermissionResolver func(r *http.Request) (rbac.Permission, error)

// RequireDynamicPermission returns a middleware that resolves the required permission
// at request time via resolver, then checks the token scope. Use this for endpoints
// where the required permission depends on request data (e.g. deploy target environment).
func RequireDynamicPermission(resolver PermissionResolver) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !config.GetConfig().RBACEnabled {
				next(w, r)
				return
			}
			perm, err := resolver(r)
			if err != nil {
				utils.WriteErrorResponse(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			if !jwtassertion.HasAllScopes(r.Context(), []string{perm.Scope()}) {
				utils.WriteErrorResponse(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next(w, r)
		}
	}
}
