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

package testsetup

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"

	"github.com/wso2/agent-manager/test/e2e/framework"
	envops "github.com/wso2/agent-manager/test/e2e/operations/environment"
)

// CleanupStaleE2EResources finds and deletes e2e projects (with the
// E2EProjectPrefix) that were created more than 1 hour ago. It deletes all
// agents within those projects first, then deletes the projects themselves.
// This is intended to be called from BeforeSuite before any tests execute.
const cutoff = 1 * time.Hour

func CleanupStaleE2EResources(client *framework.AMPClient, orgName string) {

	path := fmt.Sprintf("/api/v1/orgs/%s/projects", orgName)
	resp, err := client.Get(path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to list projects: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ginkgo.GinkgoWriter.Printf("stale cleanup: list projects returned %d, skipping\n", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to read projects response: %v\n", err)
		return
	}

	var projects framework.ProjectListResponse
	if err := json.Unmarshal(body, &projects); err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to decode projects: %v\n", err)
		return
	}

	for _, proj := range projects.Projects {
		if !strings.HasPrefix(proj.Name, framework.E2EProjectPrefix) {
			continue
		}
		if time.Since(proj.CreatedAt) < cutoff {
			continue
		}

		ginkgo.GinkgoWriter.Printf("stale cleanup: removing stale project %q (created %s)\n",
			proj.Name, proj.CreatedAt.Format(time.RFC3339))

		deleteAgentsInProject(client, orgName, proj.Name)

		// Retry project deletion — agent cleanup is async, project may still
		// report associated agents briefly after agent DELETE returns 204.
		projPath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", orgName, proj.Name)
		for attempt := 0; attempt < 5; attempt++ {
			if attempt > 0 {
				time.Sleep(3 * time.Second)
			}
			delResp, err := client.Delete(projPath)
			if err != nil {
				ginkgo.GinkgoWriter.Printf("stale cleanup: failed to delete project %s: %v\n", proj.Name, err)
				break
			}
			delResp.Body.Close()
			if delResp.StatusCode == http.StatusNoContent || delResp.StatusCode == http.StatusNotFound {
				ginkgo.GinkgoWriter.Printf("stale cleanup: deleted project %s (status %d)\n",
					proj.Name, delResp.StatusCode)
				break
			}
			if delResp.StatusCode == http.StatusConflict && attempt < 4 {
				ginkgo.GinkgoWriter.Printf("stale cleanup: project %s still has resources, retrying...\n", proj.Name)
				continue
			}
			ginkgo.GinkgoWriter.Printf("stale cleanup: delete project %s returned status %d\n",
				proj.Name, delResp.StatusCode)
			break
		}
	}

	// Order matters and mirrors the platform's referential deletion guards:
	// projects (done above) → deployment pipelines (a pipeline refuses deletion while a
	// project references it) → LLM providers → environments (an environment refuses
	// deletion while a pipeline references it).
	cleanupStaleDeploymentPipelines(client, orgName)
	cleanupStaleLLMProviders(client, orgName)
	cleanupStaleCustomEvaluators(client, orgName)
	cleanupStaleEnvironments(client, orgName)
}

// cleanupStaleCustomEvaluators deletes e2e-created custom evaluators (identifier
// prefixed with "e2e-"). Built-in evaluators are never touched. There is no creation
// timestamp on evaluators, but this runs in BeforeSuite before the current run creates
// its own (uniquely-suffixed) evaluator, so anything present is from a prior run.
// A 409 means an active monitor still references it (ErrCustomEvaluatorInUse); since
// monitors are not cleaned here, that evaluator is logged and left in place.
func cleanupStaleCustomEvaluators(client *framework.AMPClient, orgName string) {
	path := fmt.Sprintf("/api/v1/orgs/%s/evaluators", orgName)
	resp, err := client.Get(path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to list evaluators: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ginkgo.GinkgoWriter.Printf("stale cleanup: list evaluators returned %d, skipping\n", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to read evaluators response: %v\n", err)
		return
	}

	var evaluators framework.EvaluatorListResponse
	if err := json.Unmarshal(body, &evaluators); err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to decode evaluators: %v\n", err)
		return
	}

	for _, e := range evaluators.Evaluators {
		if e.IsBuiltin || !strings.HasPrefix(e.Identifier, "e2e-") {
			continue
		}

		evalPath := fmt.Sprintf("/api/v1/orgs/%s/evaluators/custom/%s", orgName, e.Identifier)
		delResp, err := client.Delete(evalPath)
		if err != nil {
			ginkgo.GinkgoWriter.Printf("stale cleanup: failed to delete custom evaluator %s: %v\n", e.Identifier, err)
			continue
		}
		delResp.Body.Close()
		logDeleteResult("custom evaluator", e.Identifier, delResp.StatusCode)
	}
}

// logDeleteResult reports a stale-cleanup DELETE outcome without misreporting a
// non-2xx response as a successful deletion: 204/404 are success, 409 means the
// resource is still referenced (guard) and is skipped, anything else is a failure.
func logDeleteResult(kind, name string, status int) {
	switch status {
	case http.StatusNoContent, http.StatusNotFound:
		ginkgo.GinkgoWriter.Printf("stale cleanup: deleted %s %s (status %d)\n", kind, name, status)
	case http.StatusConflict:
		ginkgo.GinkgoWriter.Printf("stale cleanup: %s %s still in use (status %d), skipping\n", kind, name, status)
	default:
		ginkgo.GinkgoWriter.Printf("stale cleanup: delete %s %s returned status %d (not removed)\n", kind, name, status)
	}
}

// cleanupStaleDeploymentPipelines deletes e2e-created deployment pipelines (name
// prefixed with "e2e-") that were created more than the cutoff ago. The "default"
// pipeline is never touched. These are org-scoped and are not cascaded by project
// deletion, so they orphan unless removed explicitly. Runs after project cleanup so
// the platform's "pipeline referenced by a project" guard does not block deletion.
func cleanupStaleDeploymentPipelines(client *framework.AMPClient, orgName string) {

	path := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines", orgName)
	resp, err := client.Get(path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to list deployment pipelines: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ginkgo.GinkgoWriter.Printf("stale cleanup: list deployment pipelines returned %d, skipping\n", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to read deployment pipelines response: %v\n", err)
		return
	}

	var pipelines framework.DeploymentPipelineListResponse
	if err := json.Unmarshal(body, &pipelines); err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to decode deployment pipelines: %v\n", err)
		return
	}

	for _, p := range pipelines.DeploymentPipelines {
		if p.Name == "default" || !strings.HasPrefix(p.Name, "e2e-") {
			continue
		}
		if time.Since(p.CreatedAt) < cutoff {
			continue
		}

		pipelinePath := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines/%s", orgName, p.Name)
		delResp, err := client.Delete(pipelinePath)
		if err != nil {
			ginkgo.GinkgoWriter.Printf("stale cleanup: failed to delete deployment pipeline %s: %v\n", p.Name, err)
			continue
		}
		delResp.Body.Close()
		logDeleteResult("deployment pipeline", p.Name, delResp.StatusCode)
	}
}

// cleanupStaleLLMProviders deletes e2e-created LLM providers (name prefixed with
// "e2e", case-insensitive) created more than the cutoff ago. These are org-scoped and
// are not cascaded by project or environment deletion.
func cleanupStaleLLMProviders(client *framework.AMPClient, orgName string) {

	path := fmt.Sprintf("/api/v1/orgs/%s/llm-providers", orgName)
	resp, err := client.Get(path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to list LLM providers: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ginkgo.GinkgoWriter.Printf("stale cleanup: list LLM providers returned %d, skipping\n", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to read LLM providers response: %v\n", err)
		return
	}

	var providers framework.LLMProviderListResponse
	if err := json.Unmarshal(body, &providers); err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to decode LLM providers: %v\n", err)
		return
	}

	for _, p := range providers.Providers {
		// e2e providers are named/identified "E2E ..." / "e2e-..."; match either ID or Name.
		if !strings.HasPrefix(strings.ToLower(p.Name), "e2e") && !strings.HasPrefix(strings.ToLower(p.ID), "e2e") {
			continue
		}
		if p.CreatedAt != nil && time.Since(*p.CreatedAt) < cutoff {
			continue
		}

		providerPath := fmt.Sprintf("/api/v1/orgs/%s/llm-providers/%s", orgName, p.ID)
		delResp, err := client.Delete(providerPath)
		if err != nil {
			ginkgo.GinkgoWriter.Printf("stale cleanup: failed to delete LLM provider %s: %v\n", p.ID, err)
			continue
		}
		delResp.Body.Close()
		logDeleteResult("LLM provider", p.ID, delResp.StatusCode)
	}
}

// cleanupStaleEnvironments finds environments with the E2EEnvPrefix that were
// created more than 1 hour ago and tears them down via remove-environment.sh
// (which uninstalls the gateway helm release and deletes the environment).
// The default environment is never touched. Failures are logged, not fatal,
// so a single bad teardown doesn't abort the suite.
func cleanupStaleEnvironments(client *framework.AMPClient, orgName string) {
	defaultEnv := client.Cfg().DefaultEnv

	path := fmt.Sprintf("/api/v1/orgs/%s/environments", orgName)
	resp, err := client.Get(path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to list environments: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ginkgo.GinkgoWriter.Printf("stale cleanup: list environments returned %d, skipping\n", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to read environments response: %v\n", err)
		return
	}

	var environments framework.EnvironmentListResponse
	if err := json.Unmarshal(body, &environments); err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to decode environments: %v\n", err)
		return
	}

	for _, env := range environments {
		if !strings.HasPrefix(env.Name, framework.E2EEnvPrefix) {
			continue
		}
		// Never remove the default environment, even if it somehow matches.
		if env.Name == defaultEnv {
			continue
		}
		if time.Since(env.CreatedAt) < cutoff {
			continue
		}

		ginkgo.GinkgoWriter.Printf("stale cleanup: removing stale environment %q (created %s)\n",
			env.Name, env.CreatedAt.Format(time.RFC3339))

		params := envops.FromClient(client)
		params.EnvName = env.Name
		if err := envops.RemoveEnvironment(params); err != nil {
			ginkgo.GinkgoWriter.Printf("stale cleanup: remove-environment.sh failed for %q: %v\n", env.Name, err)
			continue
		}
		ginkgo.GinkgoWriter.Printf("stale cleanup: removed environment %q\n", env.Name)
	}
}

// deleteAgentsInProject deletes all agents within a project.
func deleteAgentsInProject(client *framework.AMPClient, orgName, projName string) {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", orgName, projName)
	resp, err := client.Get(path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to list agents in %s: %v\n", projName, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var agents framework.AgentListResponse
	if err := json.Unmarshal(body, &agents); err != nil {
		return
	}

	for _, ag := range agents.Agents {
		agentPath := fmt.Sprintf("%s/%s", path, ag.Name)
		delResp, err := client.Delete(agentPath)
		if err != nil {
			ginkgo.GinkgoWriter.Printf("stale cleanup: failed to delete agent %s: %v\n", ag.Name, err)
			continue
		}
		delResp.Body.Close()
		ginkgo.GinkgoWriter.Printf("stale cleanup: deleted agent %s/%s (status %d)\n",
			projName, ag.Name, delResp.StatusCode)
	}
}
