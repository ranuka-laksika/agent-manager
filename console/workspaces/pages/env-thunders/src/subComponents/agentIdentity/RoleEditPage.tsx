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

import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Chip,
  CircularProgress,
  Form,
  IconButton,
  ListingTable,
  Stack,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Trash } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useListAgentIdentityAgents,
  useListAgentIdentityGroups,
  useGetAgentIdentityRole,
  useGetAgentIdentityRoleAssignments,
  useAddAgentIdentityRoleAssignees,
  useRemoveAgentIdentityRoleAssignees,
  useUpdateAgentIdentityRole,
  useListAgentIdentityScopes,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type AgentIdentityAgentResponse,
  type ThunderGroup,
} from "@agent-management-platform/types";
import {
  BackButton,
  EditFormSkeleton,
  EntityHeader,
} from "@agent-management-platform/shared-component";
import { useAgentLookup } from "./useAgentLookup";
import { useAssignmentDelta } from "./useAssignmentDelta";
import type { ScopeChoice } from "./scopeChoice";

type ActiveTab = "permissions" | "agents" | "groups";

// Groups assigned to a role are picked from one generous page rather than a
// dedicated "fetch all" hook — mirrors the convention used elsewhere in this
// feature area (see GroupEditPage's members picker).
const GROUPS_PAGE_SIZE = 100;

export const RoleEditPage: React.FC = () => {
  const { orgId, envName, roleId } = useParams<{
    orgId: string;
    envName: string;
    roleId: string;
  }>();
  const navigate = useNavigate();

  const [activeTab, setActiveTab] = useState<ActiveTab>("permissions");
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | undefined>();
  const [saveSuccess, setSaveSuccess] = useState(false);

  const params = { orgName: orgId, envName: envName ?? "", roleId: roleId ?? "" };

  const { data: roleData, isLoading: isLoadingRole } = useGetAgentIdentityRole(params);
  const isPermissionsReadOnly = roleData?.isReadOnly ?? false;
  const { data: assignmentsData, isLoading: isLoadingAssignments } =
    useGetAgentIdentityRoleAssignments(params);
  const { data: agentsData, isLoading: isLoadingAgents } = useListAgentIdentityAgents({
    orgName: orgId,
    envName: envName ?? "",
  });
  const { data: groupsData, isLoading: isLoadingGroups } = useListAgentIdentityGroups(
    { orgName: orgId, envName: envName ?? "" },
    { offset: 0, limit: GROUPS_PAGE_SIZE },
  );
  const { data: scopesData, isLoading: isLoadingScopes } = useListAgentIdentityScopes({
    orgName: orgId,
    envName: envName ?? "",
  });

  const { mutateAsync: addAssignees } = useAddAgentIdentityRoleAssignees();
  const { mutateAsync: removeAssignees } = useRemoveAgentIdentityRoleAssignees();
  const { mutateAsync: updateRole } = useUpdateAgentIdentityRole();

  // --- Derived server state ---
  const { agents, displayName } = useAgentLookup(agentsData?.agents ?? []);
  const allGroups: ThunderGroup[] = useMemo(() => groupsData?.groups ?? [], [groupsData]);
  const catalogScopes: ScopeChoice[] = useMemo(() => scopesData?.scopes ?? [], [scopesData]);

  const initialAgentIds: string[] = useMemo(
    () => (assignmentsData?.users ?? []).map((u) => u.id),
    [assignmentsData],
  );
  const initialGroups: ThunderGroup[] = useMemo(
    () => assignmentsData?.groups ?? [],
    [assignmentsData],
  );
  const initialScopeNames: string[] = useMemo(
    () => roleData?.permissions?.flatMap((rp) => rp.permissions) ?? [],
    [roleData],
  );

  // --- Agent tab delta tracking ---
  const agentDelta = useAssignmentDelta<AgentIdentityAgentResponse>(
    initialAgentIds,
    (a) => a.thunderAgentId as string,
  );

  // --- Group tab delta tracking ---
  const initialGroupIds = useMemo(() => initialGroups.map((g) => g.id), [initialGroups]);
  const groupDelta = useAssignmentDelta<ThunderGroup>(initialGroupIds, (g) => g.id);

  // --- Permissions tab: full selected-state approach ---
  const [selectedScopes, setSelectedScopes] = useState<ScopeChoice[]>([]);
  const hasEditedScopes = useRef(false);

  useEffect(() => {
    if (!hasEditedScopes.current && catalogScopes.length > 0) {
      const catalogByScope = new Map(catalogScopes.map((s) => [s.scope, s]));
      // A scope assigned to this role may no longer be in the environment's
      // aggregate (its owning proxy may no longer be deployed here) — keep it
      // as a placeholder so it isn't silently dropped (and dirtied) on load,
      // and stays in the payload if the role is saved.
      setSelectedScopes(
        initialScopeNames.map((name) => catalogByScope.get(name) ?? { scope: name }),
      );
    }
  }, [initialScopeNames, catalogScopes]);

  const rolesNode =
    absoluteRouteMap.children.org.children.thunderInstances.children.view.children.roles;
  const rolesPath =
    orgId && envName ? generatePath(rolesNode.path, { orgId, envName }) : "#";

  // --- Derived displayed lists ---
  const displayedAgentIds = useMemo(
    () => [...agentDelta.activeIds, ...agentDelta.pendingAddIds],
    [agentDelta.activeIds, agentDelta.pendingAddIds],
  );

  const displayedGroups = useMemo(
    () => [
      ...initialGroups.filter((g) => !groupDelta.removedIds.has(g.id)),
      ...groupDelta.pendingAdds,
    ],
    [initialGroups, groupDelta.removedIds, groupDelta.pendingAdds],
  );

  const availableAgents = useMemo(
    () => agents.filter((a) => !agentDelta.excludedIds.has(a.thunderAgentId as string)),
    [agents, agentDelta.excludedIds],
  );
  const availableGroups = useMemo(
    () => allGroups.filter((g) => !groupDelta.excludedIds.has(g.id)),
    [allGroups, groupDelta.excludedIds],
  );

  const selectedScopeNames = useMemo(
    () => new Set(selectedScopes.map((s) => s.scope)),
    [selectedScopes],
  );

  const handleAddAgent = agentDelta.handleAdd;
  const handleRemoveAgent = agentDelta.handleRemove;
  const handleAddGroup = groupDelta.handleAdd;
  const handleRemoveGroup = groupDelta.handleRemove;

  // --- Permissions handlers ---
  const handleScopesChange = (_e: React.SyntheticEvent, newValue: ScopeChoice[]) => {
    hasEditedScopes.current = true;
    setSelectedScopes(newValue);
  };

  const handleRemoveScope = (scope: string) => {
    hasEditedScopes.current = true;
    setSelectedScopes((prev) => prev.filter((s) => s.scope !== scope));
  };

  // --- Save ---
  const handleSave = async () => {
    if (!orgId || !envName || !roleId) return;
    setSaveError(undefined);
    setSaveSuccess(false);
    setIsSaving(true);
    try {
      const addAgentIds = agentDelta.pendingAdds.map((a) => a.thunderAgentId as string);
      const removeAgentIdList = [...agentDelta.removedIds];
      const addGroupIds = groupDelta.pendingAdds.map((g) => g.id);
      const removeGroupIdList = [...groupDelta.removedIds];

      // None of these calls depends on another's result, so they run
      // concurrently rather than paying for round-trips one at a time.
      await Promise.all([
        addAgentIds.length > 0
          ? addAssignees({
              params,
              body: { assignments: addAgentIds.map((id) => ({ id, type: "agent" as const })) },
            })
          : null,
        removeAgentIdList.length > 0
          ? removeAssignees({
              params,
              body: {
                assignments: removeAgentIdList.map((id) => ({ id, type: "agent" as const })),
              },
            })
          : null,
        addGroupIds.length > 0
          ? addAssignees({
              params,
              body: { assignments: addGroupIds.map((id) => ({ id, type: "group" as const })) },
            })
          : null,
        removeGroupIdList.length > 0
          ? removeAssignees({
              params,
              body: {
                assignments: removeGroupIdList.map((id) => ({ id, type: "group" as const })),
              },
            })
          : null,
        // The backend reconciles add/remove scope permissions server-side from
        // the full desired set, so a single update call is enough here.
        hasEditedScopes.current && !isPermissionsReadOnly && roleData
          ? updateRole({
              params,
              body: {
                name: roleData.name,
                description: roleData.description,
                scopes: selectedScopes.map((s) => s.scope),
              },
            })
          : null,
      ]);

      setSaveSuccess(true);
      agentDelta.reset();
      groupDelta.reset();
      hasEditedScopes.current = false;
    } catch {
      setSaveError("Failed to update role. Please try again.");
    } finally {
      setIsSaving(false);
    }
  };

  const isLoading = isLoadingRole || isLoadingAssignments || isLoadingScopes;

  const scopesDirty = useMemo(() => {
    if (isPermissionsReadOnly) return false;
    const initial = new Set(initialScopeNames);
    return (
      initial.size !== selectedScopes.length ||
      selectedScopes.some((s) => !initial.has(s.scope))
    );
  }, [isPermissionsReadOnly, initialScopeNames, selectedScopes]);

  const isDirty = scopesDirty || agentDelta.isDirty || groupDelta.isDirty;

  if (isLoading) {
    return (
      <>
        <BackButton to={rolesPath} label="Roles" />
        <EditFormSkeleton tabs={3} />
      </>
    );
  }

  return (
    <>
      <BackButton to={rolesPath} label="Roles" />
      <Stack spacing={3}>
        <EntityHeader
          fallback="R"
          name={roleData?.name ?? ""}
          subtitle={roleData?.description}
          id={roleId ?? ""}
          badge={isPermissionsReadOnly ? <Chip label="Read-only" size="small" /> : undefined}
        />
        {saveError != null && <Alert severity="error">{saveError}</Alert>}
        {saveSuccess && <Alert severity="success">Role updated successfully.</Alert>}

        <Form.Section>
          <Tabs
            value={activeTab}
            onChange={(_e, v) => setActiveTab(v as ActiveTab)}
            sx={{ borderBottom: 1, borderColor: "divider" }}
          >
            <Tab label="Permissions" value="permissions" />
            <Tab label="Agents" value="agents" />
            <Tab label="Groups" value="groups" />
          </Tabs>

          {/* ── Permissions tab ── */}
          {activeTab === "permissions" && (
            <>
              <Form.Header>Permissions</Form.Header>
              <Typography variant="body2" color="text.secondary">
                {isPermissionsReadOnly
                  ? "Permissions for predefined roles cannot be modified."
                  : "Search and select scopes to assign to this role."}
              </Typography>

              <Box sx={{ mt: 1 }}>
                {!isPermissionsReadOnly && (
                  <Form.ElementWrapper label="Add scopes" name="addScopes">
                    <Autocomplete
                      id="addScopes"
                      multiple
                      disableCloseOnSelect
                      options={catalogScopes}
                      value={selectedScopes}
                      onChange={handleScopesChange}
                      getOptionLabel={(option) => (option as ScopeChoice).scope}
                      isOptionEqualToValue={(option, value) =>
                        (option as ScopeChoice).scope === (value as ScopeChoice).scope
                      }
                      renderTags={() => null}
                      renderOption={(props, option) => (
                        <li {...props} key={(option as ScopeChoice).scope}>
                          <Box>
                            <Typography variant="body2">
                              {(option as ScopeChoice).scope}
                            </Typography>
                            {(option as ScopeChoice).description && (
                              <Typography variant="caption" color="text.secondary">
                                {(option as ScopeChoice).description}
                              </Typography>
                            )}
                          </Box>
                        </li>
                      )}
                      renderInput={(autocompleteParams) => (
                        <TextField {...autocompleteParams} placeholder="Search scopes..." />
                      )}
                      noOptionsText="No scopes in the catalog"
                      sx={{ mb: 3 }}
                    />
                  </Form.ElementWrapper>
                )}

                {selectedScopes.length === 0 ? (
                  <Typography variant="body2" color="text.secondary">
                    No scopes assigned yet.
                  </Typography>
                ) : (
                  <Stack direction="row" flexWrap="wrap" gap={1}>
                    {catalogScopes
                      .filter((s) => selectedScopeNames.has(s.scope))
                      .map((s) => (
                        <Chip
                          key={s.scope}
                          label={s.scope}
                          size="small"
                          onDelete={
                            !isPermissionsReadOnly
                              ? () => handleRemoveScope(s.scope)
                              : undefined
                          }
                        />
                      ))}
                  </Stack>
                )}
              </Box>
            </>
          )}

          {/* ── Agents tab ── */}
          {activeTab === "agents" && (
            <>
              <Form.Header>Assigned Agents</Form.Header>
              {isLoadingAgents ? (
                <CircularProgress size={20} />
              ) : (
                <>
                  <Typography variant="body2" color="text.secondary">
                    Search and add agents to this role.
                  </Typography>

                  <Box sx={{ mt: 1, mb: 2 }}>
                    <Form.ElementWrapper label="Add Agent" name="addAgent">
                      <Autocomplete
                        id="addAgent"
                        options={availableAgents}
                        getOptionLabel={(option) =>
                          (option as AgentIdentityAgentResponse).agentName
                        }
                        onChange={handleAddAgent}
                        value={null}
                        renderInput={(autocompleteParams) => (
                          <TextField {...autocompleteParams} placeholder="Search agents..." />
                        )}
                        noOptionsText="No agents available"
                      />
                    </Form.ElementWrapper>
                  </Box>

                  {displayedAgentIds.length === 0 ? (
                    <Typography variant="body2" color="text.secondary">
                      No agents assigned yet. Search and add agents above.
                    </Typography>
                  ) : (
                    <ListingTable.Container>
                      <ListingTable>
                        <ListingTable.Head>
                          <ListingTable.Row>
                            <ListingTable.Cell>Agent</ListingTable.Cell>
                            <ListingTable.Cell>Thunder Agent ID</ListingTable.Cell>
                            <ListingTable.Cell />
                          </ListingTable.Row>
                        </ListingTable.Head>
                        <ListingTable.Body>
                          {displayedAgentIds.map((id) => (
                            <ListingTable.Row key={id}>
                              <ListingTable.Cell>{displayName(id)}</ListingTable.Cell>
                              <ListingTable.Cell>{id}</ListingTable.Cell>
                              <ListingTable.Cell align="right">
                                <Tooltip title="Remove from role">
                                  <IconButton
                                    size="small"
                                    onClick={() => handleRemoveAgent(id)}
                                  >
                                    <Trash size={16} />
                                  </IconButton>
                                </Tooltip>
                              </ListingTable.Cell>
                            </ListingTable.Row>
                          ))}
                        </ListingTable.Body>
                      </ListingTable>
                    </ListingTable.Container>
                  )}
                </>
              )}
            </>
          )}

          {/* ── Groups tab ── */}
          {activeTab === "groups" && (
            <>
              <Form.Header>Assigned Groups</Form.Header>
              {isLoadingGroups ? (
                <CircularProgress size={20} />
              ) : (
                <>
                  <Typography variant="body2" color="text.secondary">
                    Search and add groups to this role.
                  </Typography>
                  {(groupsData?.total ?? 0) > GROUPS_PAGE_SIZE && (
                    <Alert severity="warning" sx={{ mt: 1 }}>
                      Showing the first {GROUPS_PAGE_SIZE} of {groupsData?.total} groups in this
                      environment. The add-group picker below only excludes groups from this page.
                    </Alert>
                  )}

                  <Box sx={{ mt: 1, mb: 2 }}>
                    <Form.ElementWrapper label="Add Group" name="addGroup">
                      <Autocomplete
                        id="addGroup"
                        options={availableGroups}
                        getOptionLabel={(option) => (option as ThunderGroup).name}
                        onChange={handleAddGroup}
                        value={null}
                        renderInput={(autocompleteParams) => (
                          <TextField {...autocompleteParams} placeholder="Search groups..." />
                        )}
                        noOptionsText="No groups available"
                      />
                    </Form.ElementWrapper>
                  </Box>

                  {displayedGroups.length === 0 ? (
                    <Typography variant="body2" color="text.secondary">
                      No groups assigned yet. Search and add groups above.
                    </Typography>
                  ) : (
                    <ListingTable.Container>
                      <ListingTable>
                        <ListingTable.Head>
                          <ListingTable.Row>
                            <ListingTable.Cell>Name</ListingTable.Cell>
                            <ListingTable.Cell>Description</ListingTable.Cell>
                            <ListingTable.Cell />
                          </ListingTable.Row>
                        </ListingTable.Head>
                        <ListingTable.Body>
                          {displayedGroups.map((group) => (
                            <ListingTable.Row key={group.id}>
                              <ListingTable.Cell>{group.name}</ListingTable.Cell>
                              <ListingTable.Cell>
                                {group.description ?? "-"}
                              </ListingTable.Cell>
                              <ListingTable.Cell align="right">
                                <Tooltip title="Remove from role">
                                  <IconButton
                                    size="small"
                                    onClick={() => handleRemoveGroup(group.id)}
                                  >
                                    <Trash size={16} />
                                  </IconButton>
                                </Tooltip>
                              </ListingTable.Cell>
                            </ListingTable.Row>
                          ))}
                        </ListingTable.Body>
                      </ListingTable>
                    </ListingTable.Container>
                  )}
                </>
              )}
            </>
          )}
        </Form.Section>

        {isDirty && (
          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              onClick={() => navigate(rolesPath)}
              disabled={isSaving}
            >
              Cancel
            </Button>
            <Button variant="contained" onClick={handleSave} disabled={isSaving}>
              {isSaving ? "Saving..." : "Save Changes"}
            </Button>
          </Stack>
        )}
      </Stack>
    </>
  );
};

export default RoleEditPage;
