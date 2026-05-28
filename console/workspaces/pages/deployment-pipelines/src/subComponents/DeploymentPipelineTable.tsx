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
  Box,
  IconButton,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { useParams } from "react-router-dom";
import { useListDeploymentPipelines } from "@agent-management-platform/api-client";
import type { DeploymentPipelineResponse } from "@agent-management-platform/types";
import { FadeIn } from "@agent-management-platform/views";

const PAGE_SIZE = 5;

interface DeploymentPipelineTableProps {
  onEditPipeline?: (pipeline: DeploymentPipelineResponse) => void;
}

export function DeploymentPipelineTable({ onEditPipeline }: DeploymentPipelineTableProps) {
  const { orgId } = useParams<{ orgId: string }>();
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);

  const { data, isLoading } = useListDeploymentPipelines({ orgName: orgId });

  const pipelines = data?.deploymentPipelines ?? [];

  const filtered = useMemo(
    () =>
      pipelines.filter(
        (p) =>
          p.name.toLowerCase().includes(search.toLowerCase()) ||
          p.displayName.toLowerCase().includes(search.toLowerCase()),
      ),
    [pipelines, search],
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
          placeholder="Search pipelines..."
          size="small"
        />

        {filtered.length === 0 ? (
          <Box display="flex" justifyContent="center" py={4}>
            <Typography variant="body2" color="text.secondary">
              {search ? "No pipelines match your search." : "No deployment pipelines found."}
            </Typography>
          </Box>
        ) : (
          <>
            <ListingTable.Container>
              <ListingTable>
                <ListingTable.Head>
                  <ListingTable.Row>
                    <ListingTable.Cell>Name</ListingTable.Cell>
                    <ListingTable.Cell>Display Name</ListingTable.Cell>
                    <ListingTable.Cell>Promotion Paths</ListingTable.Cell>
                    <ListingTable.Cell>Created</ListingTable.Cell>
                    <ListingTable.Cell align="right">Actions</ListingTable.Cell>
                  </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                  {paginated.map((pipeline) => (
                    <ListingTable.Row key={pipeline.name}>
                      <ListingTable.Cell>
                        <Typography variant="body2" fontWeight="medium">
                          {pipeline.name}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2">{pipeline.displayName}</Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2">
                          {pipeline.promotionPaths.length}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2" color="text.secondary">
                          {formatDistanceToNow(new Date(pipeline.createdAt), { addSuffix: true })}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell align="right">
                        <Tooltip title="Edit pipeline">
                          <IconButton
                            size="small"
                            onClick={() => onEditPipeline?.(pipeline)}
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
