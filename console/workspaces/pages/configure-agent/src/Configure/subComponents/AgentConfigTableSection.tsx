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

import { useMemo, useState } from "react";
import {
  Box,
  Button,
  IconButton,
  ListingTable,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { formatDistanceToNow } from "date-fns";
import {
  AlertTriangle,
  Plus,
  ServerCog,
  Trash,
} from "@wso2/oxygen-ui-icons-react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { type AgentModelConfigListItem } from "@agent-management-platform/types";
import {
  ConfigTableEmptyState,
  ConfigTableSection,
} from "./ConfigTableSection";

const COLUMN_COUNT = 4;
const ROWS_PER_PAGE_OPTIONS = [10, 25, 50];

/** Copy that distinguishes one agent-config listing (LLM, MCP, ...) from another. */
export interface AgentConfigTableLabels {
  title: string;
  searchPlaceholder: string;
  addButtonLabel: string;
  emptyTitle: string;
  emptyDescription: string;
  errorTitle: string;
  errorFallback: string;
  searchEmptyTitle: string;
  searchEmptyDescription: string;
  removeTitle: string;
  removeTooltip: string;
  removeConfirmation: (config: AgentModelConfigListItem) => string;
  removeAriaLabel: (config: AgentModelConfigListItem) => string;
}

interface AgentConfigTableSectionProps {
  /** Configs of a single type, sliced from the page's one model-config call. */
  configs: AgentModelConfigListItem[];
  isLoading: boolean;
  error: unknown;
  labels: AgentConfigTableLabels;
  /** Absolute path to the "add" page for this config type. */
  addPath: string;
  /** Builds the absolute path to a config's detail page. */
  getViewPath: (configId: string) => string;
  onRemove: (configId: string) => void;
}

/**
 * A searchable, paginated listing of an agent's model configs of a given type.
 * The LLM Configurations and MCP Servers tables are identical apart from their copy
 * and routes, so both render through this component.
 */
export function AgentConfigTableSection({
  configs,
  isLoading,
  error,
  labels,
  addPath,
  getViewPath,
  onRemove,
}: AgentConfigTableSectionProps) {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();
  const navigate = useNavigate();
  const { addConfirmation } = useConfirmationDialog();

  const [searchValue, setSearchValue] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);

  const canAdd = Boolean(orgId && projectId && agentId);

  const filteredConfigs = useMemo(() => {
    if (!searchValue.trim()) return configs;
    const lower = searchValue.toLowerCase();
    return configs.filter(
      (c) =>
        c.name.toLowerCase().includes(lower) ||
        (c.description ?? "").toLowerCase().includes(lower) ||
        c.type.toLowerCase().includes(lower),
    );
  }, [configs, searchValue]);

  const totalCount = filteredConfigs.length;

  // Pagination is client-side: the page fetches all configs in one call and
  // splits them between the tables by type. Clamp the page so it stays valid
  // when the list shrinks (e.g. after a delete or a search).
  const pageCount = Math.max(1, Math.ceil(totalCount / rowsPerPage));
  const safePage = Math.min(page, pageCount - 1);
  const paginatedConfigs = useMemo(
    () =>
      filteredConfigs.slice(
        safePage * rowsPerPage,
        safePage * rowsPerPage + rowsPerPage,
      ),
    [filteredConfigs, safePage, rowsPerPage],
  );

  const handleSearchChange = (value: string) => {
    setSearchValue(value);
    setPage(0);
  };

  const handleDelete = (config: AgentModelConfigListItem) => {
    addConfirmation({
      title: labels.removeTitle,
      description: labels.removeConfirmation(config),
      confirmButtonText: "Remove",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () => onRemove(config.uuid),
    });
  };

  const addButton = (variant: "contained" | "outlined") => (
    <Button
      component={Link}
      to={addPath}
      variant={variant}
      color="primary"
      size="small"
      startIcon={<Plus size={16} />}
      disabled={!canAdd}
    >
      {labels.addButtonLabel}
    </Button>
  );

  const toolbar = (
    <ListingTable.Toolbar
      showSearch
      searchValue={searchValue}
      onSearchChange={handleSearchChange}
      searchPlaceholder={labels.searchPlaceholder}
      actions={addButton("contained")}
    />
  );

  const tableHeader = (
    <ListingTable.Head>
      <ListingTable.Row>
        <ListingTable.Cell width="25%">Name</ListingTable.Cell>
        <ListingTable.Cell width="45%">Description</ListingTable.Cell>
        <ListingTable.Cell width="20%">Created</ListingTable.Cell>
        <ListingTable.Cell width="10%" align="right">
          Actions
        </ListingTable.Cell>
      </ListingTable.Row>
    </ListingTable.Head>
  );

  const getEmptyState = () => {
    if (error) {
      return (
        <ConfigTableEmptyState
          colSpan={COLUMN_COUNT}
          illustration={
            <Box component="span" sx={{ color: "error.main" }}>
              <AlertTriangle size={64} />
            </Box>
          }
          title={labels.errorTitle}
          description={
            error instanceof Error ? error.message : labels.errorFallback
          }
        />
      );
    }
    if (configs.length === 0) {
      return (
        <ConfigTableEmptyState
          colSpan={COLUMN_COUNT}
          illustration={<ServerCog size={64} />}
          title={labels.emptyTitle}
          description={labels.emptyDescription}
          action={addButton("outlined")}
        />
      );
    }
    return (
      <ConfigTableEmptyState
        colSpan={COLUMN_COUNT}
        illustration={<ServerCog size={64} />}
        title={labels.searchEmptyTitle}
        description={labels.searchEmptyDescription}
      />
    );
  };

  return (
    <ConfigTableSection
      title={labels.title}
      toolbar={toolbar}
      tableHeader={tableHeader}
      isLoading={isLoading}
      hasRows={filteredConfigs.length > 0}
      emptyState={getEmptyState()}
      pagination={
        <TablePagination
          rowsPerPageOptions={ROWS_PER_PAGE_OPTIONS}
          component="div"
          count={totalCount}
          rowsPerPage={rowsPerPage}
          page={safePage}
          onPageChange={(_, newPage) => setPage(newPage)}
          onRowsPerPageChange={(e) => {
            setRowsPerPage(parseInt(e.target.value, 10));
            setPage(0);
          }}
        />
      }
    >
      {paginatedConfigs.map((config) => (
        <ListingTable.Row
          key={config.uuid}
          hover
          clickable
          onClick={() => navigate(getViewPath(config.uuid))}
        >
          <ListingTable.Cell>
            <Typography variant="body2">{config.name}</Typography>
          </ListingTable.Cell>
          <ListingTable.Cell>
            <Typography variant="body2" color="text.secondary">
              {config.description ?? "—"}
            </Typography>
          </ListingTable.Cell>
          <ListingTable.Cell>
            {config.createdAt
              ? formatDistanceToNow(new Date(config.createdAt), {
                  addSuffix: true,
                })
              : "-"}
          </ListingTable.Cell>
          <ListingTable.Cell align="right">
            <Tooltip title={labels.removeTooltip}>
              <IconButton
                color="error"
                size="small"
                onClick={(e: React.MouseEvent) => {
                  e.stopPropagation();
                  handleDelete(config);
                }}
                aria-label={labels.removeAriaLabel(config)}
              >
                <Trash size={16} />
              </IconButton>
            </Tooltip>
          </ListingTable.Cell>
        </ListingTable.Row>
      ))}
    </ConfigTableSection>
  );
}
