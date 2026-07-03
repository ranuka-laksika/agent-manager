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

import React, {
  createContext,
  lazy,
  type ReactNode,
  Suspense,
  useCallback,
  useContext,
  useMemo,
  useState,
} from "react";
import { User } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
} from "@agent-management-platform/views";

const ProfilePage = lazy(() =>
  import("@agent-management-platform/profile-settings").then((m) => ({
    default: m.ProfilePage,
  }))
);

interface ProfileDrawerContextValue {
  openProfileDrawer: () => void;
  closeProfileDrawer: () => void;
}

const ProfileDrawerContext = createContext<ProfileDrawerContextValue | null>(null);

export const ProfileDrawerProvider: React.FC<{ children: ReactNode }> = ({
  children,
}) => {
  const [open, setOpen] = useState(false);

  const openProfileDrawer = useCallback(() => setOpen(true), []);
  const closeProfileDrawer = useCallback(() => setOpen(false), []);

  const ctx = useMemo(
    () => ({ openProfileDrawer, closeProfileDrawer }),
    [openProfileDrawer, closeProfileDrawer],
  );

  return (
    <ProfileDrawerContext.Provider value={ctx}>
      {children}
      <DrawerWrapper open={open} onClose={closeProfileDrawer} minWidth={560} maxWidth={700}>
        <DrawerHeader
          icon={<User size={20} />}
          title="Profile Settings"
          onClose={closeProfileDrawer}
        />
        <DrawerContent>
          <Suspense fallback={null}>
            <ProfilePage />
          </Suspense>
        </DrawerContent>
      </DrawerWrapper>
    </ProfileDrawerContext.Provider>
  );
};

export const useProfileDrawer = (): ProfileDrawerContextValue => {
  const ctx = useContext(ProfileDrawerContext);
  if (!ctx) throw new Error("useProfileDrawer must be used within ProfileDrawerProvider");
  return ctx;
};
