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

import React from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { IdentitiesOrganization } from "@agent-management-platform/identities";
import { SettingsLayout } from "./SettingsLayout";
import { ThemePage } from "./ThemePage";
import { useIdentityVisibility } from "./settingsRoutes";

export const SettingsOrganization: React.FC = () => {
  const identityVisibility = useIdentityVisibility();

  // Land on the first identity area the user can see, otherwise Appearance.
  const defaultPath = identityVisibility.users
    ? "identities/users"
    : identityVisibility.groups
      ? "identities/groups"
      : identityVisibility.roles
        ? "identities/roles"
        : "appearance/theme";

  return (
    <SettingsLayout>
      <Routes>
        <Route index element={<Navigate to={defaultPath} replace />} />
        <Route path="identities/*" element={<IdentitiesOrganization />} />
        <Route path="appearance/theme" element={<ThemePage />} />
        <Route
          path="appearance/*"
          element={<Navigate to="appearance/theme" replace />}
        />
        <Route path="*" element={<Navigate to={defaultPath} replace />} />
      </Routes>
    </SettingsLayout>
  );
};

export default SettingsOrganization;
