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

import { z } from "zod";

export const editPipelineSchema = z.object({
  displayName: z.string().min(1, "Display name is required").max(128, "Display name must be 128 characters or less"),
  description: z.string().optional(),
  promotionPaths: z
    .array(
      z.object({
        sourceEnvironmentRef: z.string().min(1, "Source environment is required"),
        targetEnvironmentRefs: z
          .array(z.object({ name: z.string().min(1) }))
          .min(1, "At least one target environment is required"),
      }),
    )
    .min(1, "At least one promotion path is required"),
});

export type EditPipelineFormValues = z.infer<typeof editPipelineSchema>;
