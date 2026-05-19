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

package api

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// RegisterLLMDeploymentRoutes registers all LLM deployment-related routes
func RegisterLLMDeploymentRoutes(mux *http.ServeMux, ctrl controllers.LLMDeploymentController) {
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/llm-providers/{providerId}/deployments", rbac.LLMProviderDeploy, ctrl.DeployLLMProvider)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/llm-providers/{providerId}/deployments/undeploy", rbac.LLMProviderDeploy, ctrl.UndeployLLMProviderDeployment)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/llm-providers/{providerId}/deployments/restore", rbac.LLMProviderDeploy, ctrl.RestoreLLMProviderDeployment)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/llm-providers/{providerId}/deployments", rbac.LLMProviderRead, ctrl.GetLLMProviderDeployments)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/llm-providers/{providerId}/deployments/{deploymentId}", rbac.LLMProviderRead, ctrl.GetLLMProviderDeployment)
	middleware.HandleFuncWithValidationAndAuthz(mux, "DELETE /orgs/{orgName}/llm-providers/{providerId}/deployments/{deploymentId}", rbac.LLMProviderDeploy, ctrl.DeleteLLMProviderDeployment)
}
