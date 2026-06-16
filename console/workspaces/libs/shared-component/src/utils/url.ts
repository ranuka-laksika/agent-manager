/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

export interface ValidateEndpointUrlOptions {
  /** Message returned when the value is empty. */
  requiredMessage?: string;
  /** Message returned when the value is not a parsable URL. */
  invalidMessage?: string;
  /** Message returned when the protocol is not http or https. */
  protocolMessage?: string;
}

/**
 * Validates an http/https endpoint URL.
 * Returns the first applicable error message, or null when the value is valid.
 */
export function validateEndpointUrl(
  value: string,
  options: ValidateEndpointUrlOptions = {},
): string | null {
  const {
    requiredMessage = "URL is required",
    invalidMessage = "Please enter a valid URL",
    protocolMessage = "URL must use http or https",
  } = options;
  const trimmed = value.trim();
  if (!trimmed) return requiredMessage;
  try {
    const parsed = new URL(trimmed);
    if (!["http:", "https:"].includes(parsed.protocol)) {
      return protocolMessage;
    }
  } catch {
    return invalidMessage;
  }
  return null;
}
