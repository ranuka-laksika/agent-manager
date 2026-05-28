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

import { useState } from "react";
import { Navigate, Route, Routes, useParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import type { DeploymentPipelineResponse } from "@agent-management-platform/types";
import { DeploymentPipelineTable } from "./subComponents/DeploymentPipelineTable";
import { EditDeploymentPipelineDrawer } from "./subComponents/EditDeploymentPipelineDrawer";

export function DeploymentPipelinesOrganization() {
  const { orgId } = useParams<{ orgId: string }>();
  const [pipelineToEdit, setPipelineToEdit] = useState<DeploymentPipelineResponse | null>(null);

  return (
    <>
      <Routes>
        <Route
          index
          element={
            <PageLayout title="Deployment Pipelines" disableIcon>
              <DeploymentPipelineTable onEditPipeline={setPipelineToEdit} />
            </PageLayout>
          }
        />
        <Route path="*" element={<Navigate to={`/org/${orgId}/deployment-pipelines`} replace />} />
      </Routes>

      {pipelineToEdit && orgId && (
        <EditDeploymentPipelineDrawer
          open={pipelineToEdit !== null}
          onClose={() => setPipelineToEdit(null)}
          pipeline={pipelineToEdit}
          orgId={orgId}
        />
      )}
    </>
  );
}
