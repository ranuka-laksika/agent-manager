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
import type { Environment } from "@agent-management-platform/types";
import { EnvironmentTable } from "./subComponents/EnvironmentTable";
import { EditEnvironmentDrawer } from "./subComponents/EditEnvironmentDrawer";
import { CreateEnvironmentDrawer } from "./subComponents/CreateEnvironmentDrawer";
import { DeleteEnvironmentDrawer } from "./subComponents/DeleteEnvironmentDrawer";

export function EnvironmentsOrganization() {
  const { orgId } = useParams<{ orgId: string }>();
  const [envToEdit, setEnvToEdit] = useState<Environment | null>(null);
  const [envToDelete, setEnvToDelete] = useState<Environment | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  return (
    <>
      <Routes>
        <Route
          index
          element={
            <PageLayout title="Environments" disableIcon>
              <EnvironmentTable
                onEditEnvironment={setEnvToEdit}
                onCreateEnvironment={() => setCreateOpen(true)}
                onDeleteEnvironment={setEnvToDelete}
              />
            </PageLayout>
          }
        />
        <Route path="*" element={<Navigate to={`/org/${orgId}/environments`} replace />} />
      </Routes>

      {orgId && (
        <CreateEnvironmentDrawer
          open={createOpen}
          onClose={() => setCreateOpen(false)}
          orgId={orgId}
        />
      )}

      {envToEdit && orgId && (
        <EditEnvironmentDrawer
          open={envToEdit !== null}
          onClose={() => setEnvToEdit(null)}
          environment={envToEdit}
          orgId={orgId}
        />
      )}

      {envToDelete && (
        <DeleteEnvironmentDrawer
          open={envToDelete !== null}
          onClose={() => setEnvToDelete(null)}
          environment={envToDelete}
        />
      )}
    </>
  );
}
