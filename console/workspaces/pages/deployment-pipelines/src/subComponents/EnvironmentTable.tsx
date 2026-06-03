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

import { type ChangeEvent, useMemo, useState } from "react";
import {
  Chip,
  IconButton,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit, Search, Server } from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { useParams } from "react-router-dom";
import { useListEnvironments } from "@agent-management-platform/api-client";
import type { Environment } from "@agent-management-platform/types";
import { FadeIn } from "@agent-management-platform/views";

const PAGE_SIZE = 5;

interface EnvironmentTableProps {
  onEditEnvironment?: (environment: Environment) => void;
}

export function EnvironmentTable({ onEditEnvironment }: EnvironmentTableProps) {
  const { orgId } = useParams<{ orgId: string }>();
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);

  const { data: environments, isLoading } = useListEnvironments({ orgName: orgId });

  const items = environments ?? [];

  const filtered = useMemo(
    () =>
      items.filter(
        (e) =>
          e.name.toLowerCase().includes(search.toLowerCase()) ||
          (e.displayName ?? "").toLowerCase().includes(search.toLowerCase()),
      ),
    [items, search],
  );

  const paginated = filtered.slice(page * PAGE_SIZE, page * PAGE_SIZE + PAGE_SIZE);

  const handleSearch = (e: ChangeEvent<HTMLInputElement>) => {
    setSearch(e.target.value);
    setPage(0);
  };

  if (isLoading) {
    return (
      <Stack spacing={1}>
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} variant="rounded" height={48} />
        ))}
      </Stack>
    );
  }

  return (
    <FadeIn>
      <Stack spacing={2}>
        <SearchBar
          value={search}
          onChange={handleSearch}
          placeholder="Search environments..."
          size="small"
        />

        {filtered.length === 0 ? (
          <ListingTable.Container>
            {search ? (
              <ListingTable.EmptyState
                illustration={<Search size={64} />}
                title="No environments match your search"
                description="Try a different keyword or clear the search filter."
              />
            ) : (
              <ListingTable.EmptyState
                illustration={<Server size={64} />}
                title="No environments yet"
                description="Environments are created and managed through your infrastructure configuration."
              />
            )}
          </ListingTable.Container>
        ) : (
          <>
            <ListingTable.Container>
              <ListingTable>
                <ListingTable.Head>
                  <ListingTable.Row>
                    <ListingTable.Cell>Environment</ListingTable.Cell>
                    <ListingTable.Cell>Data Plane</ListingTable.Cell>
                    <ListingTable.Cell>Type</ListingTable.Cell>
                    <ListingTable.Cell>Created</ListingTable.Cell>
                    <ListingTable.Cell align="right">Actions</ListingTable.Cell>
                  </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                  {paginated.map((env) => (
                    <ListingTable.Row key={env.name}>
                      <ListingTable.Cell>
                        <Typography variant="body2" fontWeight="medium">
                          {env.displayName ?? env.name}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2" color="text.secondary">
                          {env.dataplaneRef}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        {env.isProduction ? (
                          <Chip label="Production" color="error" size="small" />
                        ) : (
                          <Chip label="Non-production" size="small" />
                        )}
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2" color="text.secondary">
                          {formatDistanceToNow(new Date(env.createdAt), { addSuffix: true })}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell align="right">
                        <Tooltip title="Edit environment">
                          <IconButton
                            size="small"
                            onClick={() => onEditEnvironment?.(env)}
                          >
                            <Edit size={16} />
                          </IconButton>
                        </Tooltip>
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  ))}
                </ListingTable.Body>
              </ListingTable>
            </ListingTable.Container>
            {filtered.length > PAGE_SIZE && (
              <TablePagination
                component="div"
                count={filtered.length}
                page={page}
                rowsPerPage={PAGE_SIZE}
                rowsPerPageOptions={[PAGE_SIZE]}
                onPageChange={(_, newPage) => setPage(newPage)}
              />
            )}
          </>
        )}
      </Stack>
    </FadeIn>
  );
}
