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

package models

// ThunderInstanceResponse describes the OAuth2 identity provider that is
// provisioned per environment. Developers use these endpoints to configure
// their agents to mint and validate tokens.
type ThunderInstanceResponse struct {
	EnvName      string `json:"envName"`
	DisplayName  string `json:"displayName"`
	IsProduction bool   `json:"isProduction"`
	IssuerURL    string `json:"issuerUrl"`
	TokenURL     string `json:"tokenUrl"`
	JWKSURL      string `json:"jwksUrl"`
	Namespace    string `json:"namespace"`
}

// ThunderInstanceListResponse is the list response for thunder instances.
type ThunderInstanceListResponse struct {
	ThunderInstances []ThunderInstanceResponse `json:"thunderInstances"`
}
