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
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// RejectPublisherAudience returns middleware that rejects tokens whose audience
// matches the amp-publisher-* carve-out with 403. It runs after JWTAuth, so the
// token is already signature-verified; it is re-parsed here without verification
// only to read the audience claim.
func RejectPublisherAudience() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if IsPublisherAudience(r.Header.Get("Authorization")) {
				writeAuthError(w, http.StatusForbidden,
					"publisher tokens are not permitted on this endpoint")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IsPublisherAudience reports whether the Bearer token in authHeader carries an
// amp-publisher-* audience. The token is parsed without verifying its signature
// — callers must ensure it was already validated by JWTAuth — purely to read
// the audience claim. A missing, non-Bearer, or unparseable token reports false
// (JWTAuth is responsible for rejecting those). It lets both the REST
// RejectPublisherAudience middleware and the am-obs-mcp tool handlers apply the
// same publisher carve-out regardless of transport.
func IsPublisherAudience(authHeader string) bool {
	tokenString := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if tokenString == "" || tokenString == authHeader {
		return false
	}
	claims := jwt.MapClaims{}
	parser := jwt.NewParser()
	if _, _, err := parser.ParseUnverified(tokenString, claims); err != nil {
		return false
	}
	audiences, err := claims.GetAudience()
	if err != nil {
		return false
	}
	for _, aud := range audiences {
		if validPublisherAudPattern.MatchString(strings.TrimSpace(aud)) {
			return true
		}
	}
	return false
}
