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

import React from "react";
import { PageLayout } from "@agent-management-platform/views";
import { generatePath, Navigate, Route, Routes, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { IdentityProvidersTable } from "./subComponents/IdentityProvidersTable";

export const IdentityProvidersOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();

  const identityProvidersPath = generatePath(
    absoluteRouteMap.children.org.children.security.children.identityProviders.path,
    { orgId },
  );

  return (
    <Routes>
      {/* /security → /security/identity-providers */}
      <Route index element={<Navigate to={identityProvidersPath} replace />} />
      <Route
        path="identity-providers"
        element={
          <PageLayout
            title="Identity Providers"
            description="Token issuers configured on your gateways. Secure agent endpoints with OAuth."
            disableIcon
          >
            <IdentityProvidersTable />
          </PageLayout>
        }
      />
      <Route path="*" element={<Navigate to={identityProvidersPath} replace />} />
    </Routes>
  );
};

export default IdentityProvidersOrganization;
