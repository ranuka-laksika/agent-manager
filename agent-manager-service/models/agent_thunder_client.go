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

import (
	"time"

	"github.com/google/uuid"
)

// AgentProvisioningType identifies who ends up holding an AgentID's credential.
type AgentProvisioningType string

const (
	// AgentProvisioningTypeInternal is an agent whose workload the platform runs and
	// manages. Its secret is stored in the vault and never shown to a human.
	AgentProvisioningTypeInternal AgentProvisioningType = "internal"
	// AgentProvisioningTypeExternal is an agent the operator runs and hosts themselves.
	// Its secret is shown once and never persisted long-term by the platform.
	AgentProvisioningTypeExternal AgentProvisioningType = "external"
)

// AgentThunderStatus is the provisioning status of one binding (Section 6 of the
// AgentID architecture doc): PENDING -> IN_PROGRESS -> COMPLETED, or FAILED after
// the retry budget (5 attempts) is exhausted.
type AgentThunderStatus string

const (
	AgentThunderStatusPending    AgentThunderStatus = "pending"
	AgentThunderStatusInProgress AgentThunderStatus = "in_progress"
	AgentThunderStatusCompleted  AgentThunderStatus = "completed"
	AgentThunderStatusFailed     AgentThunderStatus = "failed"
)

// AgentThunderClient is the GORM model for the agent_thunder_clients table — the
// binding record for one agent's AgentID in one environment.
type AgentThunderClient struct {
	ID               uuid.UUID             `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OrgName          string                `gorm:"column:org_name;not null"`
	ProjectName      string                `gorm:"column:project_name;not null"`
	AgentName        string                `gorm:"column:agent_name;not null"`
	EnvironmentName  string                `gorm:"column:environment_name;not null"`
	ProvisioningType AgentProvisioningType `gorm:"column:provisioning_type;not null"`
	ThunderAgentID   string                `gorm:"column:thunder_agent_id;not null;default:''"`
	ThunderClientID  string                `gorm:"column:thunder_client_id;not null;default:''"`
	// SecretRefPath is the OpenBao KV path holding the credential. Populated for both
	// ownership types, but with different retention: permanent for internal, transient
	// (deleted on claim) for external.
	SecretRefPath string             `gorm:"column:secret_ref_path;not null;default:''"`
	Status        AgentThunderStatus `gorm:"column:status;not null;default:'pending'"`
	// RequestedBy is the calling user's own subject (from AMS's existing
	// incoming-request auth), captured for audit purposes only. Thunder's own
	// "owner" field cannot carry this — each env-Thunder is a fully isolated
	// instance with its own entity store, so a platform-Thunder user ID does
	// not resolve there (verified live: env-Thunder rejects it with AGT-1039
	// "owner not found"). This is AMS's own record of who asked, independent
	// of what Thunder's API will accept.
	RequestedBy     string     `gorm:"column:requested_by;not null;default:''"`
	AttemptCount    int        `gorm:"column:attempt_count;not null;default:0"`
	LastError       string     `gorm:"column:last_error;not null;default:''"`
	LastAttemptedAt *time.Time `gorm:"column:last_attempted_at"`
	NextRetryAt     *time.Time `gorm:"column:next_retry_at"`
	// ClaimedAt marks when an external agent's transient secret was retrieved.
	// Nil until claimed; a claimed secret cannot be re-served.
	ClaimedAt *time.Time `gorm:"column:claimed_at"`
	CreatedAt time.Time  `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt time.Time  `gorm:"column:updated_at;not null;default:NOW()"`
}

func (AgentThunderClient) TableName() string { return "agent_thunder_clients" }

// AgentIdentityEnvironmentView is one environment's AgentID binding, shaped for
// display to a caller. This is a safe, side-effect-free read: it never carries
// a secret, even for an unclaimed External binding — HasUnclaimedSecret only
// reports whether one is available. ClaimSecret (via the dedicated claim
// endpoint) is the only way to actually retrieve and consume it.
type AgentIdentityEnvironmentView struct {
	EnvironmentName  string                `json:"environmentName"`
	ProvisioningType AgentProvisioningType `json:"provisioningType"`
	Status           AgentThunderStatus    `json:"status"`
	// AgentID is Thunder's own UUID for this identity (the /agents resource ID) —
	// distinct from ClientID, which is the OAuth2 client_id. Empty until
	// provisioning actually reaches Thunder (status pending with no attempt yet).
	AgentID   string `json:"agentId,omitempty"`
	ClientID  string `json:"clientId,omitempty"`
	LastError string `json:"lastError,omitempty"`
	// HasUnclaimedSecret is true only for a completed External binding whose
	// secret has never been claimed. Internal agents and already-claimed
	// External bindings are always false.
	HasUnclaimedSecret bool `json:"hasUnclaimedSecret"`
	// RequestedBy is AMS's own record of who triggered this binding — see the
	// field doc on AgentThunderClient.RequestedBy for why this is tracked here
	// rather than via Thunder's own owner field.
	RequestedBy string `json:"requestedBy,omitempty"`
}

// AgentClaimSecretStatus is the fixed value of AgentClaimSecretResponse.Status —
// claim has exactly one successful outcome, so this isn't an enum in practice,
// just a named constant so the literal string exists in one place.
const AgentClaimSecretStatus = "claimed"

// AgentClaimSecretResponse is returned by the explicit one-time-claim endpoint
// for an External agent's secret. Calling this endpoint IS the claim: the
// first successful call returns and permanently destroys the stored secret;
// every subsequent call fails with a 404 (see utils.ErrAgentCredentialNotAvailable).
type AgentClaimSecretResponse struct {
	EnvironmentName string `json:"environmentName"`
	// AgentID is Thunder's own UUID for this identity (the /agents resource ID) —
	// distinct from ClientID, which is the OAuth2 client_id.
	AgentID      string `json:"agentId"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	Status       string `json:"status"`
}

// AgentRegenerateSecretStatus is the fixed value of AgentRegenerateSecretResponse.Status —
// regenerate has exactly one successful outcome, so this isn't an enum in
// practice, just a named constant so the literal string exists in one place.
const AgentRegenerateSecretStatus = "regenerated"

// AgentRegenerateSecretResponse is returned by the regenerate-secret endpoint.
// ClientSecret is always populated, for both Internal and External agents —
// the caller just explicitly asked to rotate this credential and already
// holds agent:update, so there is no reason to make them make a second call
// (GetAgentCredentials) just to see the value their own request produced.
// ProvisioningType is included so a caller can tell which kind of agent this
// binding belongs to without cross-referencing GetAgentIdentity.
type AgentRegenerateSecretResponse struct {
	EnvironmentName  string                `json:"environmentName"`
	ProvisioningType AgentProvisioningType `json:"provisioningType"`
	ClientID         string                `json:"clientId"`
	ClientSecret     string                `json:"clientSecret"`
	Status           string                `json:"status"`
}

// AgentRevokeSecretStatus is the fixed value of AgentRevokeSecretResponse.Status —
// revoke has exactly one successful outcome, so this isn't an enum in practice,
// just a named constant so the literal string exists in one place.
const AgentRevokeSecretStatus = "revoked"

// AgentRevokeSecretResponse is returned by the revoke-secret endpoint. There is
// never a clientSecret here — revoke is a kill switch, not a rotation; an
// explicit regenerate afterward is required to get a usable credential again.
type AgentRevokeSecretResponse struct {
	EnvironmentName string `json:"environmentName"`
	ClientID        string `json:"clientId"`
	Status          string `json:"status"`
}

// AgentCredentialsResponse is returned by the Internal-agent-only credentials
// endpoint. Unlike every other identity response, ClientSecret is always
// populated and retrieval is repeatable — Internal agents have no other way to
// obtain their own credential today (Gateway Binding, which will inject it
// into the workload automatically, is a later phase).
type AgentCredentialsResponse struct {
	EnvironmentName string `json:"environmentName"`
	// AgentID is Thunder's own UUID for this identity (the /agents resource ID) —
	// distinct from ClientID, which is the OAuth2 client_id.
	AgentID      string `json:"agentId"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}
