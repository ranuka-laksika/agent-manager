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
import { ListingTable, Skeleton, Stack } from "@wso2/oxygen-ui";

interface ListingSkeletonRowsProps {
  /** Number of placeholder rows to render. Defaults to 5. */
  rows?: number;
}

/**
 * Placeholder rows for the identity listing tables (Users / Roles / Groups),
 * which all share a "two text columns + a right-aligned actions cell" shape.
 * Rendered inside ListingTable.Body while the list query is loading so the
 * table chrome stays stable instead of flashing a centered spinner.
 */
export const ListingSkeletonRows: React.FC<ListingSkeletonRowsProps> = ({
  rows = 5,
}) => (
  <>
    {Array.from({ length: rows }).map((_, index) => (
      <ListingTable.Row key={index}>
        <ListingTable.Cell>
          <Skeleton variant="text" width="50%" />
        </ListingTable.Cell>
        <ListingTable.Cell>
          <Skeleton variant="text" width="70%" />
        </ListingTable.Cell>
        <ListingTable.Cell align="right">
          <Stack direction="row" spacing={0.5} justifyContent="flex-end">
            <Skeleton variant="circular" width={28} height={28} />
            <Skeleton variant="circular" width={28} height={28} />
          </Stack>
        </ListingTable.Cell>
      </ListingTable.Row>
    ))}
  </>
);
