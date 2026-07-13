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

package thundersvc

// KV key names inside one AgentID credential entry, written by
// services/agent_thunder_provisioning_service.go via secretmanagersvc.
// Exported because the credential-injection path
// (services/agent_identity_injection_service.go) points a SecretReference CR
// at this same KV entry, and the SecretKeyRef in the pod spec must name the
// exact key the value was stored under.
const (
	AgentSecretKeyClientID     = "client_id"
	AgentSecretKeyClientSecret = "client_secret"
)
