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

import { Sidebar } from "@wso2/oxygen-ui";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";

export interface NavigationItem {
  label: string;
  icon?: ReactNode;
  href?: string;
  isActive?: boolean;
  type: 'item';
}
export interface NavigationSection {
  title: string;
  items: Array<NavigationItem>;
  icon?: ReactNode;
  type: 'section';
}

interface LeftNavigationProps {
  collapsed: boolean;
  activeItem: string;
  mainItems: NavigationItem[];
  groupedItems: NavigationSection[];
}

// Render each item as a real anchor (`<a>`) so clicking opens the link —
// enabling middle-click / open-in-new-tab and proper link semantics, rather
// than navigating from the Sidebar's onSelect callback.
function renderItem(item: NavigationItem) {
  return (
    <Sidebar.Item
      id={item.label}
      key={item.label}
      link={item.href ? <Link to={item.href} /> : undefined}
    >
      <Sidebar.ItemIcon>{item.icon}</Sidebar.ItemIcon>
      <Sidebar.ItemLabel>{item.label}</Sidebar.ItemLabel>
    </Sidebar.Item>
  );
}

export function LeftNavigation({
  collapsed,
  activeItem,
  mainItems,
  groupedItems,
}: LeftNavigationProps) {
  return (
    <Sidebar collapsed={collapsed} activeItem={activeItem} width={280}>
      <Sidebar.Nav>
        <Sidebar.Category>{mainItems.map(renderItem)}</Sidebar.Category>
        {groupedItems.map((group) => (
          <Sidebar.Category key={group.title}>
            <Sidebar.CategoryLabel>{group.title}</Sidebar.CategoryLabel>
            {group.items.map(renderItem)}
          </Sidebar.Category>
        ))}
      </Sidebar.Nav>
    </Sidebar>
  );
}
