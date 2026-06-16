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

import yaml from "js-yaml";
import { PolicyDefinition, ParameterSchema } from "./types";

interface RawPolicyYaml {
  name?: unknown;
  version?: unknown;
  description?: unknown;
  parameters?: unknown;
  systemParameters?: unknown;
}

const PARAMETER_SCHEMA_TYPES = new Set<string>([
  "object",
  "array",
  "string",
  "boolean",
  "number",
  "integer",
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function createEmptyParameterSchema(): ParameterSchema {
  return {
    type: "object",
    properties: {},
  };
}

function normalizeSchema(value: unknown): ParameterSchema | undefined {
  if (!isRecord(value)) {
    return undefined;
  }

  if (
    typeof value.type !== "string" ||
    !PARAMETER_SCHEMA_TYPES.has(value.type)
  ) {
    return undefined;
  }

  const schema = { ...value, type: value.type } as ParameterSchema;

  if (value.properties !== undefined || value.type === "object") {
    schema.properties = normalizeProperties(value.properties);
  } else {
    delete schema.properties;
  }

  if (value.items !== undefined) {
    const items = normalizeSchema(value.items);
    if (items) {
      schema.items = items;
    } else {
      delete schema.items;
    }
  }

  return schema;
}

function normalizeProperties(value: unknown): Record<string, ParameterSchema> {
  if (!isRecord(value)) {
    return {};
  }

  return Object.entries(value).reduce<Record<string, ParameterSchema>>(
    (properties, [key, schema]) => {
      const normalizedSchema = normalizeSchema(schema);
      if (normalizedSchema) {
        properties[key] = normalizedSchema;
      }
      return properties;
    },
    {},
  );
}

function normalizeRootSchema(value: unknown): ParameterSchema {
  return normalizeSchema(value) ?? createEmptyParameterSchema();
}

export function parsePolicyYaml(yamlContent: string): PolicyDefinition {
  const parsed = yaml.load(yamlContent);

  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("Policy definition YAML must be an object");
  }

  const policy = parsed as RawPolicyYaml;

  if (typeof policy.name !== "string" || !policy.name) {
    throw new Error("Policy definition must have a name");
  }

  if (typeof policy.version !== "string" || !policy.version) {
    throw new Error(
      "Policy definition must have a version. eg: 1.0.0",
    );
  }

  const parameters = normalizeRootSchema(policy.parameters);
  const systemParameters = normalizeRootSchema(policy.systemParameters);

  return {
    name: policy.name,
    version: policy.version,
    description:
      typeof policy.description === "string" ? policy.description : "",
    parameters,
    systemParameters,
  };
}
