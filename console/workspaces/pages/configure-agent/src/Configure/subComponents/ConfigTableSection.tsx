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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { type ReactNode } from "react";
import {
  Box,
  ListingTable,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";

interface ConfigTableEmptyStateProps {
  /** Centered illustration shown above the title. */
  illustration: ReactNode;
  title: string;
  description: string;
  /** Optional action (e.g. an "Add" button) rendered in the middle of the box. */
  action?: ReactNode;
  /** Number of columns to span so the empty state stays centered. */
  colSpan: number;
}

/**
 * A centered empty-state row for a {@link ListingTable}. Used to communicate an
 * empty list, a failed load, or an empty search result, and can optionally
 * surface a primary action in the middle of the box.
 */
export function ConfigTableEmptyState({
  illustration,
  title,
  description,
  action,
  colSpan,
}: ConfigTableEmptyStateProps) {
  return (
    <ListingTable.Row>
      <ListingTable.Cell colSpan={colSpan}>
        <Box sx={{ textAlign: "center", py: 4 }}>
          <Box sx={{ mb: 2 }}>{illustration}</Box>
          <Typography variant="body2" fontWeight={500} gutterBottom>
            {title}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {description}
          </Typography>
          {action ? <Box sx={{ mt: 2 }}>{action}</Box> : null}
        </Box>
      </ListingTable.Cell>
    </ListingTable.Row>
  );
}

interface ConfigTableSectionProps {
  /** Section heading rendered above the table. */
  title: string;
  /** Toolbar (search + actions). */
  toolbar: ReactNode;
  /**
   * Whether to render the toolbar. Keep it visible whenever there is data to
   * search — even when the current filter matches nothing — so the search input
   * never disappears out from under the user.
   */
  showToolbar: boolean;
  tableHeader: ReactNode;
  isLoading: boolean;
  /** Whether the filtered list has at least one row. */
  hasRows: boolean;
  /** Rendered inside the table body when {@link hasRows} is false. */
  emptyState: ReactNode;
  /** The table rows, rendered when {@link hasRows} is true. */
  children: ReactNode;
  /** Optional pagination footer. */
  pagination?: ReactNode;
}

/**
 * Shared shell for an agent configuration listing (LLM providers, MCP servers,
 * ...). Keeps the toolbar, loading skeleton, table header and empty-state
 * handling consistent across sections. The caller controls toolbar visibility
 * via {@link ConfigTableSectionProps.showToolbar}: it stays visible while there
 * is data to search so a zero-result filter can't hide the search input, and is
 * hidden only when the list is genuinely empty (so the empty state — including
 * any primary action — is centered in the box).
 */
export function ConfigTableSection({
  title,
  toolbar,
  showToolbar,
  tableHeader,
  isLoading,
  hasRows,
  emptyState,
  children,
  pagination,
}: ConfigTableSectionProps) {
  return (
    <Stack spacing={2}>
      <Typography variant="h6">{title}</Typography>
      <ListingTable.Container>
        {showToolbar && toolbar}
        {isLoading ? (
          <Stack spacing={1} sx={{ m: 2 }}>
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} variant="rounded" height={56} />
            ))}
          </Stack>
        ) : (
          <ListingTable>
            {tableHeader}
            <ListingTable.Body>
              {hasRows ? children : emptyState}
            </ListingTable.Body>
          </ListingTable>
        )}
        {pagination}
      </ListingTable.Container>
    </Stack>
  );
}
