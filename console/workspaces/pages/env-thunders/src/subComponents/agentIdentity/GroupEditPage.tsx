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

import React, { useMemo, useState } from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
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
  useGetAgentIdentityGroup,
  useGetAgentIdentityGroupMembers,
  useGetAgentIdentityGroupRoles,
  useAddAgentIdentityGroupMembers,
  useRemoveAgentIdentityGroupMembers,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type AgentIdentityAgentResponse,
  type ThunderRole,
} from "@agent-management-platform/types";
import { BackButton } from "./components/BackButton";
import { EditFormSkeleton } from "./components/EditFormSkeleton";
import { EntityHeader } from "./components/EntityHeader";
import { useAgentLookup } from "./useAgentLookup";

type ActiveTab = "agents" | "roles";

// Group members are paginated server-side, but agent-identity groups are
// expected to stay small, so one generous page stands in for "all members"
// (mirrors the simpler `limit: 100` picker convention used elsewhere in the
// identities pages, rather than adding a dedicated "fetch all" hook).
const MEMBERS_PAGE_SIZE = 100;

export const GroupEditPage: React.FC = () => {
  const { orgId, envName, groupId } = useParams<{
    orgId: string;
    envName: string;
    groupId: string;
  }>();
  const navigate = useNavigate();

  const [activeTab, setActiveTab] = useState<ActiveTab>("agents");

  const params = { orgName: orgId, envName: envName ?? "", groupId: groupId ?? "" };

  const { data: groupData, isLoading: isLoadingGroup } = useGetAgentIdentityGroup(params);
  const { data: membersData, isLoading: isLoadingMembers } = useGetAgentIdentityGroupMembers(
    params,
    { offset: 0, limit: MEMBERS_PAGE_SIZE },
  );
  const {
    data: rolesData,
    isLoading: isLoadingRoles,
    isError: isRolesError,
  } = useGetAgentIdentityGroupRoles(params);
  const { data: agentsData, isLoading: isLoadingAgents } = useListAgentIdentityAgents({
    orgName: orgId,
    envName: envName ?? "",
  });

  const { mutateAsync: addMembers } = useAddAgentIdentityGroupMembers();
  const { mutateAsync: removeMembers } = useRemoveAgentIdentityGroupMembers();

  const { agents, displayName } = useAgentLookup(agentsData?.agents ?? []);

  const initialMemberIds: string[] = useMemo(
    () => (membersData?.members ?? []).map((m) => m.id),
    [membersData],
  );
  const roles: ThunderRole[] = useMemo(() => rolesData?.roles ?? [], [rolesData]);

  const [pendingAdds, setPendingAdds] = useState<AgentIdentityAgentResponse[]>([]);
  const [removedIds, setRemovedIds] = useState<Set<string>>(new Set());

  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | undefined>();
  const [saveSuccess, setSaveSuccess] = useState(false);

  const groupsNode =
    absoluteRouteMap.children.org.children.thunderInstances.children.view.children.groups;
  const groupsPath =
    orgId && envName ? generatePath(groupsNode.path, { orgId, envName }) : "#";

  const pageMemberIds = useMemo(
    () => initialMemberIds.filter((id) => !removedIds.has(id)),
    [initialMemberIds, removedIds],
  );

  const availableAgents = useMemo(() => {
    const excluded = new Set([
      ...initialMemberIds.filter((id) => !removedIds.has(id)),
      ...pendingAdds.map((a) => a.thunderAgentId as string),
    ]);
    return agents.filter((a) => !excluded.has(a.thunderAgentId as string));
  }, [agents, initialMemberIds, pendingAdds, removedIds]);

  const handleAddAgent = (
    _e: React.SyntheticEvent,
    value: AgentIdentityAgentResponse | null,
  ) => {
    if (!value?.thunderAgentId) return;
    if (removedIds.has(value.thunderAgentId)) {
      setRemovedIds((prev) => {
        const next = new Set(prev);
        next.delete(value.thunderAgentId as string);
        return next;
      });
    } else {
      setPendingAdds((prev) => [...prev, value]);
    }
  };

  const handleRemoveAgent = (thunderAgentId: string) => {
    if (pendingAdds.find((a) => a.thunderAgentId === thunderAgentId)) {
      setPendingAdds((prev) => prev.filter((a) => a.thunderAgentId !== thunderAgentId));
    } else {
      setRemovedIds((prev) => new Set([...prev, thunderAgentId]));
    }
  };

  const handleSave = async () => {
    if (!orgId || !envName || !groupId) return;
    setSaveError(undefined);
    setSaveSuccess(false);
    setIsSaving(true);
    try {
      const idsToAdd = pendingAdds
        .map((a) => a.thunderAgentId as string)
        .filter((id) => !initialMemberIds.includes(id));
      const idsToRemove = [...removedIds];
      await Promise.all([
        idsToAdd.length > 0 ? addMembers({ params, body: { agentIds: idsToAdd } }) : null,
        idsToRemove.length > 0 ? removeMembers({ params, body: { agentIds: idsToRemove } }) : null,
      ]);
      setSaveSuccess(true);
      setPendingAdds([]);
      setRemovedIds(new Set());
    } catch {
      setSaveError("Failed to update group members. Please try again.");
    } finally {
      setIsSaving(false);
    }
  };

  const isLoading = isLoadingGroup || isLoadingMembers || isLoadingAgents;

  if (isLoading) {
    return (
      <>
        <BackButton to={groupsPath} label="Groups" />
        <EditFormSkeleton tabs={2} />
      </>
    );
  }

  const isDirty = pendingAdds.length > 0 || removedIds.size > 0;

  return (
    <>
      <BackButton to={groupsPath} label="Groups" />
      <Stack spacing={3}>
        <EntityHeader
          fallback="G"
          name={groupData?.name ?? ""}
          subtitle={groupData?.description}
          id={groupId ?? ""}
        />
        {saveError != null && <Alert severity="error">{saveError}</Alert>}
        {saveSuccess && (
          <Alert severity="success">Group updated successfully.</Alert>
        )}

        <Form.Section>
          <Tabs
            value={activeTab}
            onChange={(_e, v) => setActiveTab(v as ActiveTab)}
            sx={{ borderBottom: 1, borderColor: "divider" }}
          >
            <Tab label="Agents" value="agents" />
            <Tab label="Roles" value="roles" />
          </Tabs>

          {/* ── Agents tab ── */}
          {activeTab === "agents" && (
            <>
              <Form.Header>Agents</Form.Header>
              <Typography variant="body2" color="text.secondary">
                Search and add agents to this group.
              </Typography>
              {(membersData?.total ?? 0) > MEMBERS_PAGE_SIZE && (
                <Alert severity="warning" sx={{ mt: 1 }}>
                  Showing the first {MEMBERS_PAGE_SIZE} of {membersData?.total} members. The
                  add-agent picker below only excludes agents from this page.
                </Alert>
              )}

              <Box sx={{ mt: 1 }}>
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

              {pendingAdds.length > 0 && (
                <Box mb={2}>
                  <Typography variant="body2" fontWeight={500} mb={1}>
                    Pending additions (unsaved)
                  </Typography>
                  <ListingTable.Container>
                    <ListingTable>
                      <ListingTable.Head>
                        <ListingTable.Row>
                          <ListingTable.Cell>Agent</ListingTable.Cell>
                          <ListingTable.Cell>Project</ListingTable.Cell>
                          <ListingTable.Cell />
                        </ListingTable.Row>
                      </ListingTable.Head>
                      <ListingTable.Body>
                        {pendingAdds.map((agent) => (
                          <ListingTable.Row key={agent.thunderAgentId}>
                            <ListingTable.Cell>{agent.agentName}</ListingTable.Cell>
                            <ListingTable.Cell>{agent.projectName}</ListingTable.Cell>
                            <ListingTable.Cell align="right">
                              <Tooltip title="Remove from group">
                                <IconButton
                                  size="small"
                                  onClick={() =>
                                    handleRemoveAgent(agent.thunderAgentId as string)
                                  }
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
                </Box>
              )}

              {pageMemberIds.length === 0 && pendingAdds.length === 0 ? (
                <Typography variant="body2" color="text.secondary">
                  No members yet. Search and add agents above.
                </Typography>
              ) : pageMemberIds.length > 0 ? (
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
                      {pageMemberIds.map((id) => (
                        <ListingTable.Row key={id}>
                          <ListingTable.Cell>{displayName(id)}</ListingTable.Cell>
                          <ListingTable.Cell>{id}</ListingTable.Cell>
                          <ListingTable.Cell align="right">
                            <Tooltip title="Remove from group">
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
              ) : null}
            </>
          )}

          {/* ── Roles tab ── */}
          {activeTab === "roles" && (
            <>
              <Form.Header>Assigned Roles</Form.Header>
              <Typography variant="body2" color="text.secondary">
                Roles currently assigned to this group. Manage role assignments
                from the Roles page.
              </Typography>

              <Box sx={{ mt: 1 }}>
                {isLoadingRoles ? (
                  <CircularProgress size={20} />
                ) : isRolesError ? (
                  <Typography variant="body2" color="error">
                    Failed to load roles. Please try again.
                  </Typography>
                ) : roles.length === 0 ? (
                  <Typography variant="body2" color="text.secondary">
                    No roles assigned to this group.
                  </Typography>
                ) : (
                  <ListingTable.Container>
                    <ListingTable>
                      <ListingTable.Head>
                        <ListingTable.Row>
                          <ListingTable.Cell>Name</ListingTable.Cell>
                          <ListingTable.Cell>Description</ListingTable.Cell>
                        </ListingTable.Row>
                      </ListingTable.Head>
                      <ListingTable.Body>
                        {roles.map((role) => (
                          <ListingTable.Row key={role.id}>
                            <ListingTable.Cell>{role.name}</ListingTable.Cell>
                            <ListingTable.Cell>
                              {role.description ?? "-"}
                            </ListingTable.Cell>
                          </ListingTable.Row>
                        ))}
                      </ListingTable.Body>
                    </ListingTable>
                  </ListingTable.Container>
                )}
              </Box>
            </>
          )}
        </Form.Section>

        {isDirty && (
          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              onClick={() => navigate(groupsPath)}
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

export default GroupEditPage;
