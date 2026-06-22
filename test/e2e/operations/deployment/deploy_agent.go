package deployment

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// DeployAgent deploys (or redeploys) an agent with the given image and env vars.
func DeployAgent(g Gomega, client *framework.AMPClient, orgName, projName, agentName string, req framework.DeployAgentRequest) framework.DeployAgentResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments",
		orgName, projName, agentName)

	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "deploy agent request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 202)

	return framework.DecodeBody[framework.DeployAgentResponse](g, resp)
}

// GetDeploymentDetails retrieves the deployment details for an agent.
// Returns a map of environment name to deployment details.
func GetDeploymentDetails(g Gomega, client *framework.AMPClient, orgName, projName, agentName string) map[string]framework.DeploymentDetailsResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments",
		orgName, projName, agentName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get deployments request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[map[string]framework.DeploymentDetailsResponse](g, resp)
}

// DeploymentStatus returns the deployment status of the agent in the given
// environment, or "" if there is no such deployment or the lookup fails. It is
// non-asserting (no test failure on absence), intended for idempotent setup
// decisions such as "is this shared agent already usable?".
func DeploymentStatus(client *framework.AMPClient, orgName, projName, agentName, envName string) string {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments",
		orgName, projName, agentName)

	resp, err := client.Get(path)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	var deps map[string]framework.DeploymentDetailsResponse
	if err := json.Unmarshal(body, &deps); err != nil {
		return ""
	}
	return deps[envName].Status
}

// IsActive reports whether the agent's deployment in the given environment is
// active. Non-asserting; see DeploymentStatus.
func IsActive(client *framework.AMPClient, orgName, projName, agentName, envName string) bool {
	return DeploymentStatus(client, orgName, projName, agentName, envName) == "active"
}
