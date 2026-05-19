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

package api

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func registerAgentRoutes(mux *http.ServeMux, ctrl controllers.AgentController) {
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/projects/{projName}/agents", rbac.AgentCreate, ctrl.CreateAgent)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects/{projName}/agents", rbac.AgentRead, ctrl.ListAgents)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/utils/generate-name", rbac.AgentCreate, ctrl.GenerateName)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}", rbac.AgentRead, ctrl.GetAgent)
	middleware.HandleFuncWithValidationAndAuthz(mux, "PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}", rbac.AgentUpdate, ctrl.UpdateAgentBasicInfo)
	middleware.HandleFuncWithValidationAndAuthz(mux, "PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/build-parameters", rbac.AgentUpdate, ctrl.UpdateAgentBuildParameters)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/resource-configs", rbac.AgentRead, ctrl.GetAgentResourceConfigs)
	middleware.HandleFuncWithValidationAndAuthz(mux, "PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/resource-configs", rbac.AgentUpdate, ctrl.UpdateAgentResourceConfigs)
	middleware.HandleFuncWithValidationAndAuthz(mux, "DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}", rbac.AgentDelete, ctrl.DeleteAgent)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds", rbac.AgentBuild, ctrl.BuildAgent)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds", rbac.AgentRead, ctrl.ListAgentBuilds)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds/{buildName}", rbac.AgentRead, ctrl.GetBuild)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds/{buildName}/build-logs", rbac.AgentRead, ctrl.GetBuildLogs)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments", rbac.AgentDeployNonProduction, ctrl.DeployAgent)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments", rbac.AgentRead, ctrl.GetAgentDeployments)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments/state", rbac.AgentSuspend, ctrl.UpdateDeploymentState)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/endpoints", rbac.AgentRead, ctrl.GetAgentEndpoints)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/configurations", rbac.AgentRead, ctrl.GetAgentConfigurations)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/metrics", rbac.AgentRead, ctrl.GetAgentMetrics)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/runtime-logs", rbac.AgentRead, ctrl.GetAgentRuntimeLogs)
}
