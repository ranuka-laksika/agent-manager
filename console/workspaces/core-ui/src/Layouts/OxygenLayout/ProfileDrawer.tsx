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

import { User } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
} from "@agent-management-platform/views";
import { ProfilePage } from "./ProfilePage";

export interface ProfileDrawerProps {
  open: boolean;
  onClose: () => void;
}

export function ProfileDrawer({ open, onClose }: ProfileDrawerProps) {
  return (
    <DrawerWrapper open={open} onClose={onClose} minWidth={560} maxWidth={700}>
      <DrawerHeader icon={<User size={20} />} title="Profile Settings" onClose={onClose} />
      <DrawerContent>
        {open && <ProfilePage />}
      </DrawerContent>
    </DrawerWrapper>
  );
}
