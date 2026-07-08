/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  type ChangeEvent,
  type MouseEvent,
  Fragment,
  useMemo,
  useState,
} from "react";
import {
  Alert,
  Avatar,
  IconButton,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Copy,
  KeyRound,
  Search,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useListEnvironments,
  useListThunderInstances,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type ThunderInstanceResponse,
} from "@agent-management-platform/types";
import { useSnackBar } from "@agent-management-platform/views";

interface EnvironmentSection {
  key: string;
  title: string;
  instance: ThunderInstanceResponse;
}

const PROVIDER_NAME = "Thunder";

const monoEllipsisSx = {
  fontFamily: "monospace",
  color: "text.secondary",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  display: "block",
} as const;

const matchesQuery = (instance: ThunderInstanceResponse, query: string) =>
  [PROVIDER_NAME, instance.issuerUrl, instance.tokenUrl]
    .join(" ")
    .toLowerCase()
    .includes(query);

export function ThunderInstancesTable() {
  const navigate = useNavigate();
  const { orgId } = useParams<{ orgId: string }>();
  const { pushSnackBar } = useSnackBar();
  const [searchQuery, setSearchQuery] = useState("");

  const {
    data,
    isLoading: instancesLoading,
    error: instancesError,
  } = useListThunderInstances({ orgName: orgId });
  const { data: environments, isLoading: environmentsLoading } = useListEnvironments({
    orgName: orgId,
  });

  const isLoading = instancesLoading || environmentsLoading;
  // Only block the table on a failure to load instances themselves — the
  // environments query only enriches titles/grouping, and `environments ?? []`
  // already degrades gracefully if it fails.
  const error = instancesError;

  const instances = useMemo(() => data?.thunderInstances ?? [], [data]);

  // Group providers per environment, ordered by the environment list, with
  // instances whose environment is missing from that list appended so nothing
  // silently disappears. Environments without a provider are omitted.
  const sections = useMemo<EnvironmentSection[]>(() => {
    const instancesByEnv = new Map(instances.map((i) => [i.envName, i]));
    const grouped: EnvironmentSection[] = [];
    const knownEnvs = new Set<string>();
    for (const env of environments ?? []) {
      knownEnvs.add(env.name);
      const instance = instancesByEnv.get(env.name);
      if (instance) {
        grouped.push({
          key: env.name,
          title: env.displayName || env.name,
          instance,
        });
      }
    }
    for (const instance of instances) {
      if (!knownEnvs.has(instance.envName)) {
        grouped.push({
          key: instance.envName,
          title: instance.displayName || instance.envName,
          instance,
        });
      }
    }
    const query = searchQuery.trim().toLowerCase();
    return query
      ? grouped.filter((section) => matchesQuery(section.instance, query))
      : grouped;
  }, [environments, instances, searchQuery]);

  const handleCopy = (e: MouseEvent<HTMLButtonElement>, value: string) => {
    e.stopPropagation();
    navigator.clipboard
      .writeText(value)
      .then(() => {
        pushSnackBar({
          message: "Token endpoint copied to clipboard",
          type: "success",
        });
      })
      .catch(() => {
        pushSnackBar({
          message: "Failed to copy token endpoint",
          type: "error",
        });
      });
  };

  const handleRowClick = (instance: ThunderInstanceResponse) => {
    navigate(
      generatePath(
        absoluteRouteMap.children.org.children.thunderInstances.children.view
          .path,
        { orgId: orgId ?? "", envName: instance.envName },
      ),
    );
  };

  const toolbar = (
    <SearchBar
      placeholder="Search identity providers..."
      size="small"
      fullWidth
      value={searchQuery}
      onChange={(e: ChangeEvent<HTMLInputElement>) =>
        setSearchQuery(e.target.value)
      }
      disabled={isLoading}
    />
  );

  if (isLoading) {
    return (
      <ListingTable.Container disablePaper>
        <Stack spacing={1} mt={1}>
          {Array.from({ length: 3 }).map((_: unknown, i: number) => (
            <Stack
              key={i}
              direction="row"
              alignItems="center"
              spacing={2}
              sx={{
                px: 2,
                py: 1.5,
                borderRadius: 1,
                border: "1px solid",
                borderColor: "divider",
                bgcolor: "background.paper",
              }}
            >
              <Skeleton variant="circular" width={36} height={36} />
              <Skeleton
                variant="text"
                width={160}
                height={20}
                sx={{ flex: 1 }}
              />
              <Skeleton variant="rounded" width={96} height={24} />
              <Skeleton variant="rounded" width={24} height={24} />
            </Stack>
          ))}
        </Stack>
      </ListingTable.Container>
    );
  }

  if (error) {
    return (
      <Alert severity="error" icon={<AlertTriangle size={18} />}>
        Failed to load identity providers. Please try again.
      </Alert>
    );
  }

  if (instances.length === 0) {
    return (
      <ListingTable.Container>
        <ListingTable.EmptyState
          illustration={<KeyRound size={64} />}
          title="No identity providers"
          description="Add an environment first. Each environment automatically gets a Thunder identity provider."
        />
      </ListingTable.Container>
    );
  }

  // The toolbar stays mounted at a stable tree position across the states
  // below — swapping its parent structure between renders would remount the
  // search input and drop its focus.
  return (
    <Stack spacing={1}>
      {toolbar}
      {sections.length === 0 ? (
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<Search size={64} />}
            title="No identity providers found"
            description="Try a different keyword or clear the search filter."
          />
        </ListingTable.Container>
      ) : (
        <Stack pt={3}>
          <ListingTable.Container disablePaper>
            <ListingTable variant="card">
              <ListingTable.Head>
                <ListingTable.Row>
                  <ListingTable.Cell width="240px">
                    Identity Provider
                  </ListingTable.Cell>
                  <ListingTable.Cell>Issuer</ListingTable.Cell>
                  <ListingTable.Cell>Token Endpoint</ListingTable.Cell>
                </ListingTable.Row>
              </ListingTable.Head>
              <ListingTable.Body>
                {sections.map(({ key, title, instance }) => (
                  <Fragment key={key}>
                    <ListingTable.Row>
                      {/* "&&" outranks the table's descendant padding rule on
                        .MuiTableCell-root, which beats a plain sx padding. */}
                      <ListingTable.Cell
                        colSpan={3}
                        sx={{ "&&": { border: 0, p: 0 } }}
                      >
                        <Typography variant="overline" color="text.secondary">
                          {title}
                        </Typography>
                      </ListingTable.Cell>
                    </ListingTable.Row>
                    <ListingTable.Row
                      variant="card"
                      hover
                      clickable
                      onClick={() => handleRowClick(instance)}
                    >
                      <ListingTable.Cell>
                        <ListingTable.CellIcon
                          icon={
                            <Avatar
                              sx={{
                                width: 28,
                                height: 28,
                                bgcolor: "primary.main",
                                color: "primary.contrastText",
                              }}
                            >
                              <KeyRound size={16} />
                            </Avatar>
                          }
                          primary={PROVIDER_NAME}
                          secondary="System identity provider"
                        />
                      </ListingTable.Cell>

                      <ListingTable.Cell>
                        <Typography
                          variant="caption"
                          sx={{ ...monoEllipsisSx, maxWidth: 280 }}
                        >
                          {instance.issuerUrl}
                        </Typography>
                      </ListingTable.Cell>

                      <ListingTable.Cell>
                        <Stack direction="row" alignItems="center" spacing={1}>
                          <Typography
                            variant="caption"
                            sx={{ ...monoEllipsisSx, maxWidth: 320 }}
                          >
                            {instance.tokenUrl}
                          </Typography>
                          <Tooltip title="Copy token endpoint">
                            <IconButton
                              size="small"
                              onClick={(e) => handleCopy(e, instance.tokenUrl)}
                            >
                              <Copy size={14} />
                            </IconButton>
                          </Tooltip>
                        </Stack>
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  </Fragment>
                ))}
              </ListingTable.Body>
            </ListingTable>
          </ListingTable.Container>
        </Stack>
      )}
    </Stack>
  );
}
