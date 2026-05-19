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

func RegisterGatewayRoutes(mux *http.ServeMux, ctrl controllers.GatewayController) {
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/gateways", rbac.GatewayCreate, ctrl.RegisterGateway)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/gateways", rbac.GatewayRead, ctrl.ListGateways)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/gateways/{gatewayID}", rbac.GatewayRead, ctrl.GetGateway)
	middleware.HandleFuncWithValidationAndAuthz(mux, "PUT /orgs/{orgName}/gateways/{gatewayID}", rbac.GatewayUpdate, ctrl.UpdateGateway)
	middleware.HandleFuncWithValidationAndAuthz(mux, "DELETE /orgs/{orgName}/gateways/{gatewayID}", rbac.GatewayDelete, ctrl.DeleteGateway)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}", rbac.GatewayUpdate, ctrl.AssignGatewayToEnvironment)
	middleware.HandleFuncWithValidationAndAuthz(mux, "DELETE /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}", rbac.GatewayUpdate, ctrl.RemoveGatewayFromEnvironment)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/gateways/{gatewayID}/environments", rbac.GatewayRead, ctrl.GetGatewayEnvironments)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/gateways/{gatewayID}/health", rbac.GatewayRead, ctrl.CheckGatewayHealth)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/gateways/{gatewayID}/tokens", rbac.GatewayTokenManage, ctrl.ListGatewayTokens)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/gateways/{gatewayID}/tokens", rbac.GatewayTokenManage, ctrl.RotateGatewayToken)
	middleware.HandleFuncWithValidationAndAuthz(mux, "DELETE /orgs/{orgName}/gateways/{gatewayID}/tokens/{tokenID}", rbac.GatewayTokenManage, ctrl.RevokeGatewayToken)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/gateways/status", rbac.GatewayRead, ctrl.GetGatewayStatus)
}
