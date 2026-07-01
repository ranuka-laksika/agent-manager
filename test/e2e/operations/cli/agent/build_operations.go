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

// amctl agent build read commands (list/get/logs) as thin, assertion-backed
// operations over the amctl harness. Lives in package cliagent alongside
// agent_operations.go. Decode structs mirror the CLI's BuildsListResponse /
// BuildDetailsResponse / LogsResponse (only the fields specs assert on).
//
// The mutating `build create` command is intentionally not wrapped here: it
// triggers a new build that becomes the agent's latest, which the sibling
// `agent deploy` specs would then pick up (they deploy the latest build and
// reject in-progress ones). These read helpers run against the CLI-owned
// agent's existing build without disturbing those specs.
package cliagent

import (
	"strconv"
	"time"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework/amctl"
)

// BuildSummary is the subset of a `agent build list` entry (the CLI's
// BuildResponse) we assert on.
type BuildSummary struct {
	AgentName   string    `json:"agentName"`
	ProjectName string    `json:"projectName"`
	BuildName   string    `json:"buildName"`
	ImageId     string    `json:"imageId"`
	Status      string    `json:"status"`
	StartedAt   time.Time `json:"startedAt"`
}

// BuildList is the data shape of `agent build list --json` (BuildsListResponse).
type BuildList struct {
	Builds []BuildSummary `json:"builds"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
	Total  int            `json:"total"`
}

// Names returns the build names in the list, for membership assertions.
func (l BuildList) Names() []string {
	names := make([]string, 0, len(l.Builds))
	for _, b := range l.Builds {
		names = append(names, b.BuildName)
	}
	return names
}

// BuildStep is one entry of BuildDetailsResponse.Steps.
type BuildStep struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// BuildDetails is the subset of `agent build get` (the CLI's
// BuildDetailsResponse) we assert on.
type BuildDetails struct {
	AgentName   string      `json:"agentName"`
	ProjectName string      `json:"projectName"`
	BuildName   string      `json:"buildName"`
	ImageId     string      `json:"imageId"`
	Status      string      `json:"status"`
	StartedAt   time.Time   `json:"startedAt"`
	Steps       []BuildStep `json:"steps"`
}

// BuildLogEntry is one entry of the CLI's LogsResponse.
type BuildLogEntry struct {
	Log       string    `json:"log"`
	LogLevel  string    `json:"logLevel"`
	Timestamp time.Time `json:"timestamp"`
}

// BuildLogs is the data shape of `agent build logs --json` (LogsResponse).
type BuildLogs struct {
	Logs       []BuildLogEntry `json:"logs"`
	TookMs     float32         `json:"tookMs"`
	TotalCount int             `json:"totalCount"`
}

// ListAgentBuilds runs `agent build list <agent>`.
func ListAgentBuilds(g Gomega, h *amctl.Harness, org, project, agent string) BuildList {
	return amctl.DecodeData[BuildList](g, h.Run(
		"agent", "build", "list", agent,
		"--org", org, "--project", project, "--json"))
}

// ListAgentBuildsPaged runs `agent build list <agent> --limit --offset`.
func ListAgentBuildsPaged(g Gomega, h *amctl.Harness, org, project, agent string, limit, offset int) BuildList {
	return amctl.DecodeData[BuildList](g, h.Run(
		"agent", "build", "list", agent,
		"--limit", strconv.Itoa(limit), "--offset", strconv.Itoa(offset),
		"--org", org, "--project", project, "--json"))
}

// GetAgentBuild runs `agent build get <agent> <build>`.
func GetAgentBuild(g Gomega, h *amctl.Harness, org, project, agent, build string) BuildDetails {
	return amctl.DecodeData[BuildDetails](g, h.Run(
		"agent", "build", "get", agent, build,
		"--org", org, "--project", project, "--json"))
}

// GetAgentBuildExpectError runs `agent build get` expecting a non-zero exit
// (e.g. an unknown build name) and returns the error envelope.
func GetAgentBuildExpectError(g Gomega, h *amctl.Harness, org, project, agent, build string) amctl.EnvelopeError {
	return h.Run("agent", "build", "get", agent, build,
		"--org", org, "--project", project, "--json").ExpectError(g)
}

// GetAgentBuildLogs runs `agent build logs <agent> <build>` for a named build.
func GetAgentBuildLogs(g Gomega, h *amctl.Harness, org, project, agent, build string) BuildLogs {
	return amctl.DecodeData[BuildLogs](g, h.Run(
		"agent", "build", "logs", agent, build,
		"--org", org, "--project", project, "--json"))
}

// GetAgentLatestBuildLogs runs `agent build logs <agent>` with no build name,
// which the CLI resolves to the agent's latest build.
func GetAgentLatestBuildLogs(g Gomega, h *amctl.Harness, org, project, agent string) BuildLogs {
	return amctl.DecodeData[BuildLogs](g, h.Run(
		"agent", "build", "logs", agent,
		"--org", org, "--project", project, "--json"))
}

// GetAgentBuildLogsExpectError runs `agent build logs` expecting a non-zero exit
// (e.g. an unknown build name) and returns the error envelope.
func GetAgentBuildLogsExpectError(g Gomega, h *amctl.Harness, org, project, agent, build string) amctl.EnvelopeError {
	return h.Run("agent", "build", "logs", agent, build,
		"--org", org, "--project", project, "--json").ExpectError(g)
}
