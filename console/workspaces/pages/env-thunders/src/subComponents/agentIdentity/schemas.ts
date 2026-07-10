/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { z } from "zod";

// Zod schema for the agent-identity group/role creation flows. Mirrors
// pages/identities/src/forms/schemas.ts so these forms validate identically
// (field-level on change, whole-form on submit) via the shared
// useFormValidation hook. Groups and roles share the same name/description
// shape, so both aliases point at one schema instead of two copies.

export interface NameDescriptionFormValues {
  name: string;
  description?: string;
}

export const nameDescriptionSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "Name is required")
    .max(50, "Name must be at most 50 characters"),
  description: z.string().trim().optional(),
});

export type CreateAgentIdentityGroupFormValues = NameDescriptionFormValues;
export const createAgentIdentityGroupSchema = nameDescriptionSchema;

export type CreateAgentIdentityRoleFormValues = NameDescriptionFormValues;
export const createAgentIdentityRoleSchema = nameDescriptionSchema;
