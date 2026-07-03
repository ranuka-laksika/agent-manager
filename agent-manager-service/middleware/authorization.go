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
	"errors"
	"log/slog"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ResolverError is returned by a PermissionResolver to signal an expected failure
// with a specific HTTP status code and message. Use NewResolverInputError for bad
// request data (400) and NewResolverForbiddenError for explicit deny (403).
// Any other error type from a resolver is treated as an internal failure (500).
type ResolverError struct {
	StatusCode int
	Message    string
}

func (e *ResolverError) Error() string { return e.Message }

// NewResolverInputError returns a ResolverError that maps to 400 Bad Request.
func NewResolverInputError(msg string) *ResolverError {
	return &ResolverError{StatusCode: http.StatusBadRequest, Message: msg}
}

// NewResolverForbiddenError returns a ResolverError that maps to 403 Forbidden.
func NewResolverForbiddenError(msg string) *ResolverError {
	return &ResolverError{StatusCode: http.StatusForbidden, Message: msg}
}

// RequireOrgMatch returns a middleware that:
//  1. Validates token carries ouId (required for both cloud and on-prem).
//  2. Validates path {orgName} parameter matches token's OuHandle.
//  3. Injects ResolvedOrg into the request context for handlers to use.
func RequireOrgMatch(resolver OrgResolver) func(http.HandlerFunc) http.HandlerFunc {
	return requireOrgMatch(resolver, false)
}

// RequireOrgMatchAllowRootOU behaves like RequireOrgMatch but additionally allows
// a client-credentials token issued to the configured root/admin OU
// (config.RootOUHandle) to access any org's route. ResolvedOrg is set to the
// PATH org (resolved via OrgResolver), not the token's admin OU, so downstream
// handlers operate on the correct tenant. Use only for system-client
// endpoints (currently: gateway registration during org bootstrap) — do not
// apply broadly to user-facing routes.
func RequireOrgMatchAllowRootOU(resolver OrgResolver) func(http.HandlerFunc) http.HandlerFunc {
	return requireOrgMatch(resolver, true)
}

func requireOrgMatch(resolver OrgResolver, allowRootOU bool) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims := jwtassertion.GetTokenClaims(r.Context())
			if claims == nil {
				slog.Warn("RequireOrgMatch rejected", "reason", "missing token claims", "path", r.URL.Path)
				utils.WriteErrorResponse(w, http.StatusForbidden, "missing token claims")
				return
			}
			if claims.OuId == "" || claims.OuHandle == "" {
				slog.Warn("RequireOrgMatch rejected", "reason", "missing ou identity in token", "sub", claims.Sub, "path", r.URL.Path)
				utils.WriteErrorResponse(w, http.StatusForbidden, "missing ou identity in token")
				return
			}

			// Validate path orgName matches token's OuHandle
			pathOrg := r.PathValue(utils.PathParamOrgName)
			if pathOrg != claims.OuHandle {
				if allowRootOU && claims.OuHandle == config.GetConfig().RootOUHandle {
					ouid, err := resolver.ResolveOUID(r.Context(), pathOrg)
					if err != nil {
						var re *ResolverError
						if errors.As(err, &re) {
							utils.WriteErrorResponse(w, re.StatusCode, re.Message)
						} else {
							utils.WriteErrorResponse(w, http.StatusInternalServerError, "internal error resolving organization")
						}
						return
					}
					slog.Info("RequireOrgMatch: root OU token granted cross-org access", "sub", claims.Sub, "pathOrg", pathOrg)
					ctx := WithResolvedOrg(r.Context(), ResolvedOrg{
						OuHandle: pathOrg,
						OUID:     ouid,
					})
					next(w, r.WithContext(ctx))
					return
				}
				slog.Warn("RequireOrgMatch rejected", "reason", "invalid organization access", "sub", claims.Sub, "tokenOu", claims.OuHandle, "pathOrg", pathOrg)
				utils.WriteErrorResponse(w, http.StatusForbidden, "invalid organization access")
				return
			}

			ctx := WithResolvedOrg(r.Context(), ResolvedOrg{
				OuHandle: claims.OuHandle,
				OUID:     claims.OuId,
			})
			next(w, r.WithContext(ctx))
		}
	}
}

// RequirePermission returns a middleware that checks the request token carries the
// required amp: scope. When RBAC_ENABLED=false the check is skipped entirely,
// allowing zero-downtime rollout.
func RequirePermission(perm rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return requirePermission(perm, false)
}

// RequirePermissionAllowRootOU behaves like RequirePermission but additionally
// allows a client-credentials token issued to the configured root/admin OU
// (config.RootOUHandle) through regardless of scope. Use only for system-client
// endpoints (currently: gateway registration during org bootstrap) — do not
// apply broadly to user-facing routes.
func RequirePermissionAllowRootOU(perm rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return requirePermission(perm, true)
}

func requirePermission(perm rbac.Permission, allowRootOU bool) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !config.GetConfig().RBACEnabled {
				next(w, r)
				return
			}
			if allowRootOU {
				if claims := jwtassertion.GetTokenClaims(r.Context()); claims != nil && claims.OuHandle == config.GetConfig().RootOUHandle {
					next(w, r)
					return
				}
			}
			if !jwtassertion.HasAllScopes(r.Context(), []string{perm.Scope()}) {
				utils.WriteErrorResponse(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next(w, r)
		}
	}
}

// RequireAnyPermission returns a middleware that passes if the token carries at least
// one of the given permissions (OR semantics). Use this for endpoints that are
// legitimately reachable via multiple roles (e.g. environments read needed by both
// the environment manager and the LLM-provider viewer).
func RequireAnyPermission(perms ...rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !config.GetConfig().RBACEnabled {
				next(w, r)
				return
			}
			for _, perm := range perms {
				if jwtassertion.HasAllScopes(r.Context(), []string{perm.Scope()}) {
					next(w, r)
					return
				}
			}
			utils.WriteErrorResponse(w, http.StatusForbidden, "insufficient permissions")
		}
	}
}

// PermissionResolver resolves the required permission at request time.
// Return *ResolverError to signal expected failures with a specific status code
// (use NewResolverInputError for 400, NewResolverForbiddenError for 403).
// Any other error is treated as an internal failure and results in a 500 response.
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
				var re *ResolverError
				if errors.As(err, &re) {
					utils.WriteErrorResponse(w, re.StatusCode, re.Message)
				} else {
					utils.WriteErrorResponse(w, http.StatusInternalServerError, "internal error resolving permission")
				}
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
