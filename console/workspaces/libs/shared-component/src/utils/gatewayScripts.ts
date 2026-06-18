/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { globalConfig } from "@agent-management-platform/types";

/**
 * Fallback gateway version when `globalConfig.gatewayVersion` is not injected
 * (e.g. local dev without a runtime config). Released builds have the real
 * version substituted at deploy time.
 */
export const DEFAULT_GATEWAY_VERSION = "v0.9.0";

/** Local helm chart path for the gateway extension, relative to a repo clone root. */
export const LOCAL_GATEWAY_EXTENSION_CHART_REF =
  "deployments/helm-charts/wso2-amp-api-platform-gateway-extension";

/** The configured gateway version, e.g. "v0.15.0" (carries a leading "v"). */
export function getGatewayVersion(): string {
  return globalConfig.gatewayVersion?.trim() || DEFAULT_GATEWAY_VERSION;
}

/** Gateway version without the leading "v", for contexts that need bare semver. */
export function getGatewayVersionHelm(): string {
  const v = getGatewayVersion();
  return v.startsWith("v") ? v.slice(1) : v;
}

/**
 * Git ref used to fetch deployment scripts from raw.githubusercontent.com.
 * Release tags are `amp/vX.Y.Z`, so a versioned build pins to that tag. Dev
 * placeholder versions (e.g. `0.0.0-dev`) have no matching tag, so fall back to
 * `main`.
 */
export function getScriptRef(): string {
  const version = getGatewayVersion();
  return version.includes("dev") ? "main" : `amp/${version}`;
}

/** Full raw URL for a deployment script pinned to the current release ref. */
export function getRawScriptUrl(scriptName: string): string {
  return `https://raw.githubusercontent.com/wso2/agent-manager/${getScriptRef()}/deployments/scripts/${scriptName}`;
}
