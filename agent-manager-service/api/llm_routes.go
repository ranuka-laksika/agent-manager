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

// RegisterLLMRoutes registers all LLM-related routes
func RegisterLLMRoutes(mux *http.ServeMux, ctrl controllers.LLMController) {
	// LLM Provider Templates
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/llm-provider-templates", rbac.LLMProviderTemplateCreate, ctrl.CreateLLMProviderTemplate)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/llm-provider-templates", rbac.LLMProviderTemplateRead, ctrl.ListLLMProviderTemplates)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/llm-provider-templates/{templateId}", rbac.LLMProviderTemplateRead, ctrl.GetLLMProviderTemplate)
	middleware.HandleFuncWithValidationAndAuthz(mux, "PUT /orgs/{orgName}/llm-provider-templates/{templateId}", rbac.LLMProviderTemplateUpdate, ctrl.UpdateLLMProviderTemplate)
	middleware.HandleFuncWithValidationAndAuthz(mux, "DELETE /orgs/{orgName}/llm-provider-templates/{templateId}", rbac.LLMProviderTemplateDelete, ctrl.DeleteLLMProviderTemplate)

	// LLM Providers
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/llm-providers", rbac.LLMProviderCreate, ctrl.CreateLLMProvider)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/llm-providers", rbac.LLMProviderRead, ctrl.ListLLMProviders)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/llm-providers/{providerId}", rbac.LLMProviderRead, ctrl.GetLLMProvider)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/llm-providers/{providerId}/llm-proxies", rbac.LLMProxyRead, ctrl.ListLLMProxiesByProvider)
	middleware.HandleFuncWithValidationAndAuthz(mux, "PUT /orgs/{orgName}/llm-providers/{providerId}", rbac.LLMProviderUpdate, ctrl.UpdateLLMProvider)
	middleware.HandleFuncWithValidationAndAuthz(mux, "PUT /orgs/{orgName}/llm-providers/{providerId}/catalog", rbac.LLMProviderUpdate, ctrl.UpdateLLMProviderCatalogStatus)
	middleware.HandleFuncWithValidationAndAuthz(mux, "DELETE /orgs/{orgName}/llm-providers/{providerId}", rbac.LLMProviderDelete, ctrl.DeleteLLMProvider)

	// LLM Proxies
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/projects/{projName}/llm-proxies", rbac.LLMProxyCreate, ctrl.CreateLLMProxy)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects/{projName}/llm-proxies", rbac.LLMProxyRead, ctrl.ListLLMProxies)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/projects/{projName}/llm-proxies/{proxyId}", rbac.LLMProxyRead, ctrl.GetLLMProxy)
	middleware.HandleFuncWithValidationAndAuthz(mux, "PUT /orgs/{orgName}/projects/{projName}/llm-proxies/{proxyId}", rbac.LLMProxyUpdate, ctrl.UpdateLLMProxy)
	middleware.HandleFuncWithValidationAndAuthz(mux, "DELETE /orgs/{orgName}/projects/{projName}/llm-proxies/{proxyId}", rbac.LLMProxyDelete, ctrl.DeleteLLMProxy)
}
