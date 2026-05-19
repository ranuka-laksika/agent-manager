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

func registerInfraRoutes(mux *http.ServeMux, ctrl controllers.InfraResourceController) {
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs", rbac.OrgView, ctrl.ListOrganizations)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}", rbac.OrgView, ctrl.GetOrganization)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/data-planes", rbac.DataPlaneRead, ctrl.GetDataplanes)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/deployment-pipelines", rbac.DeploymentPipelineRead, ctrl.ListOrgDeploymentPipelines)
	// NOTE: /orgs/{orgName}/environments routes are now registered in environment_routes.go
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects", rbac.ProjectRead, ctrl.ListProjects)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/projects", rbac.ProjectCreate, ctrl.CreateProject)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects/{projName}", rbac.ProjectRead, ctrl.GetProject)
	middleware.HandleFuncWithValidationAndAuthz(mux, "PUT /orgs/{orgName}/projects/{projName}", rbac.ProjectUpdate, ctrl.UpdateProject)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects/{projName}/deployment-pipeline", rbac.DeploymentPipelineRead, ctrl.GetProjectDeploymentPipeline)
	middleware.HandleFuncWithValidationAndAuthz(mux, "DELETE /orgs/{orgName}/projects/{projName}", rbac.ProjectDelete, ctrl.DeleteProject)
}
