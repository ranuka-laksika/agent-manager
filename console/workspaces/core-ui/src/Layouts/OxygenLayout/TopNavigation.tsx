import {
  useListAgents,
  useListOrganizations,
  useListProjects,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  Box,
  ButtonBase,
  Chip,
  ComplexSelect,
  Header,
  IconButton,
  Menu,
  MenuItem,
  Stack,
  useTheme,
} from "@wso2/oxygen-ui";
import {
  Building2,
  ChevronRight,
  Plus,
  X,
} from "@wso2/oxygen-ui-icons-react";
import { useMemo, useState, type ElementType } from "react";
import {
  generatePath,
  Link,
  useNavigate,
  useParams,
  type LinkProps,
} from "react-router-dom";
import { useActiveAgentPage, useActiveOrgPage, useActiveProjectPage } from "./path-map";

/**
 * Adapts a target URL into the props that make a menu item render as a real
 * anchor (`<a>`) so clicking it opens the link — enabling middle-click /
 * open-in-new-tab and proper link semantics. `ComplexSelect.MenuItem` forwards
 * arbitrary props to the underlying MUI MenuItem but its prop type doesn't
 * include router Link props, so we pass them through a typed object.
 */
const asLink = (to: LinkProps["to"]): { component: ElementType; to: LinkProps["to"] } => ({
  component: Link,
  to,
});

export function TopNavigation() {
  const navigate = useNavigate();
  const theme = useTheme();
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const commonOrgPages = useActiveOrgPage();
  const commonProjectPages = useActiveProjectPage();
  const commonAgentPages = useActiveAgentPage();
  const [projectAnchorEl, setProjectAnchorEl] = useState<null | HTMLElement>(
    null,
  );
  const projectMenuOpen = Boolean(projectAnchorEl);

  const [agentAnchorEl, setAgentAnchorEl] = useState<null | HTMLElement>(null);
  const agentMenuOpen = Boolean(agentAnchorEl);

  // Get all organizations
  const { data: organizations } = useListOrganizations();
  const selectedOrganization = useMemo(() => {
    return organizations?.organizations?.find(
      (organization) => organization.name === orgId,
    );
  }, [organizations, orgId]);

  // Get all projects for the organization
  const { data: projects } = useListProjects({
    orgName: orgId,
  });

  const selectedProject = useMemo(() => {
    return projects?.projects?.find((project) => project.name === projectId);
  }, [projects, projectId]);

  // Get all agents for the project
  const { data: agents } = useListAgents({
    orgName: orgId,
    projName: projectId,
  });

  const selectedAgent = useMemo(() => {
    return agents?.agents?.find((agent) => agent.name === agentId);
  }, [agents, agentId]);

  return (
    <>
      <Header.Switchers showDivider={false}>
        {organizations?.organizations && (
          <>
            {selectedOrganization && organizations.total > 1 && (
              <ComplexSelect
                value={orgId}
                size="small"
                sx={{ minWidth: 180 }}
                label="Organizations"
                renderValue={() => (
                  <ComplexSelect.MenuItem.Text
                    primary={selectedOrganization?.displayName}
                  />
                )}
              >
                {organizations.organizations.map((organization) => (
                  <ComplexSelect.MenuItem
                    key={organization.name}
                    value={organization.name}
                    {...asLink(
                      generatePath(absoluteRouteMap.children.org.path, {
                        orgId: organization.name,
                      }) + (commonOrgPages ? `/${commonOrgPages}` : ""),
                    )}
                  >
                    <ComplexSelect.MenuItem.Text
                      primary={organization.displayName ?? organization.name}
                    />
                  </ComplexSelect.MenuItem>
                ))}
              </ComplexSelect>
            )}
            {selectedOrganization && organizations.total == 1 && (
              <>
                <ButtonBase
                 aria-label="Go to organization"
                 {...asLink(
                      generatePath(absoluteRouteMap.children.org.path, {
                        orgId: selectedOrganization.name,
                      }) + (commonOrgPages ? `/${commonOrgPages}` : ""),
                    )}

                sx={{
                  color: theme.vars?.palette.text.primary,
                  border: `1px solid ${theme.vars?.palette.divider}`,
                  p: theme.spacing(1.75, 1.75),
                  borderRadius: theme.spacing(1),
                  "&:hover": {
                    border: `1px solid ${theme.vars?.palette.text.primary}`,
                  },
                }}>
                  <Building2 size={22} />
                </ButtonBase>
              </>
            )}

          </>
        )}

        {projects?.projects && (
          <>
            {selectedProject ? (
              <Box position="relative">
                <ComplexSelect
                  value={projectId}
                  size="small"
                  sx={{ minWidth: 180 }}
                  label="Projects"
                  renderValue={() => (
                    <>
                      <ComplexSelect.MenuItem.Text
                        primary={selectedProject?.displayName}
                      />
                    </>
                  )}
                >
                  <ComplexSelect.MenuItem
                    {...asLink(
                      generatePath(
                        absoluteRouteMap.children.org.children.newProject.path,
                        { orgId },
                      ),
                    )}
                  >
                    <ComplexSelect.MenuItem.Icon>
                      <Plus size={20} />
                    </ComplexSelect.MenuItem.Icon>
                    <ComplexSelect.MenuItem.Text primary="Create a Project" />
                  </ComplexSelect.MenuItem>
                  {projects.projects.map((project) => (
                    <ComplexSelect.MenuItem
                      key={project.name}
                      value={project.name}
                      {...asLink(
                        generatePath(
                          absoluteRouteMap.children.org.children.projects.path,
                          { orgId, projectId: project.name },
                        ) + (commonProjectPages ? `/${commonProjectPages}` : ""),
                      )}
                    >
                      <ComplexSelect.MenuItem.Text
                        primary={project.displayName}
                      />
                    </ComplexSelect.MenuItem>
                  ))}
                </ComplexSelect>
                <Box position="absolute" right={0} top={-2}>
                  <IconButton
                    size="small"
                    sx={{
                      color: theme.vars?.palette.text.disabled,
                    }}
                    onClick={() => {
                      navigate(
                        generatePath(absoluteRouteMap.children.org.path, {
                          orgId,
                        }),
                      );
                    }}
                  >
                    <X size={12} />
                  </IconButton>
                </Box>
              </Box>
            ) : (
              <>
                <IconButton
                  onClick={(e) => setProjectAnchorEl(e.currentTarget)}
                  size="small"
                  sx={{
                    "& .chevron-icon": {
                      transform: projectMenuOpen ? "rotate(90deg)" : "rotate(0deg)",
                      transition: "transform 0.2s",
                    },
                    borderRadius: theme.spacing(1),
                    color: theme.vars?.palette.text.primary,
                    border: `1px solid ${theme.vars?.palette.divider}`,
                    p: theme.spacing(1, 1),
                  }}
                >
                  <ChevronRight size={20} className="chevron-icon" />
                </IconButton>
                <Menu
                  anchorEl={projectAnchorEl}
                  open={projectMenuOpen}
                  onClose={() => setProjectAnchorEl(null)}
                >
                  <MenuItem
                    onClick={() => setProjectAnchorEl(null)}
                    {...asLink(
                      generatePath(
                        absoluteRouteMap.children.org.children.newProject.path,
                        { orgId },
                      ),
                    )}
                  >
                    <Plus size={20} style={{ marginRight: theme.spacing(1) }} />
                    Create a Project
                  </MenuItem>
                  {projects.projects.map((project) => (
                    <MenuItem
                      key={project.name}
                      onClick={() => setProjectAnchorEl(null)}
                      {...asLink(
                        generatePath(
                          absoluteRouteMap.children.org.children.projects.path,
                          { orgId, projectId: project.name },
                        ),
                      )}
                    >
                      {project.displayName}
                    </MenuItem>
                  ))}
                </Menu>
              </>
            )}
          </>
        )}

        {agents?.agents && (
          <>
            {selectedAgent ? (
              <Box position="relative">
                <ComplexSelect
                  value={agentId}
                  size="small"
                  label="Agents"
                  sx={{ minWidth: 180 }}
                  renderValue={() => (
                    <>
                      <ComplexSelect.MenuItem.Text
                        primary={selectedAgent?.displayName}
                      />
                    </>
                  )}
                >
                  <ComplexSelect.MenuItem
                    {...asLink(
                      generatePath(
                        absoluteRouteMap.children.org.children.projects.children
                          .newAgent.path,
                        { orgId, projectId },
                      ),
                    )}
                  >
                    <ComplexSelect.MenuItem.Icon>
                      <Plus size={20} />
                    </ComplexSelect.MenuItem.Icon>
                    <ComplexSelect.MenuItem.Text primary="Create an Agent" />
                  </ComplexSelect.MenuItem>
                  {agents.agents.map((agent) => (
                    <ComplexSelect.MenuItem
                      key={agent.name}
                      value={agent.name}
                      {...asLink(
                        generatePath(
                          absoluteRouteMap.children.org.children.projects
                            .children.agents.path,
                          { orgId, projectId, agentId: agent.name },
                        ) + (commonAgentPages ? `/${commonAgentPages}` : ""),
                      )}
                    >
                      <ComplexSelect.MenuItem.Text
                        primary={
                          <Stack direction="row" gap={1} alignItems="center">
                            {agent.displayName}
                            {agent.provisioning.type === "external" && (
                              <Chip
                                label={"External"}
                                size="small"
                                variant="outlined"
                              />
                            )}
                          </Stack>
                        }
                      />
                    </ComplexSelect.MenuItem>
                  ))}
                </ComplexSelect>
                <Box position="absolute" right={0} top={-2}>
                  <IconButton
                    size="small"
                    sx={{
                      color: theme.vars?.palette.text.disabled,
                    }}
                    onClick={() => {
                      navigate(
                        generatePath(
                          absoluteRouteMap.children.org.children.projects.path,
                          { orgId, projectId },
                        ),
                      );
                    }}
                  >
                    <X size={12} />
                  </IconButton>
                </Box>
              </Box>
            ) : (
              <>
                <IconButton
                  onClick={(e) => setAgentAnchorEl(e.currentTarget)}
                  size="small"
                  sx={{
                    "& .chevron-icon": {
                      transform: agentMenuOpen ? "rotate(90deg)" : "rotate(0deg)",
                      transition: "transform 0.2s",
                    },
                    borderRadius: theme.spacing(1),
                    color: theme.vars?.palette.text.primary,
                    border: `1px solid ${theme.vars?.palette.divider}`,
                    p: theme.spacing(1, 1),
                  }}
                >
                  <ChevronRight size={20} className="chevron-icon" />
                </IconButton>
                <Menu
                  anchorEl={agentAnchorEl}
                  open={agentMenuOpen}
                  onClose={() => setAgentAnchorEl(null)}
                >
                  <MenuItem
                    onClick={() => setAgentAnchorEl(null)}
                    {...asLink(
                      generatePath(
                        absoluteRouteMap.children.org.children.projects.children
                          .newAgent.path,
                        { orgId, projectId },
                      ),
                    )}
                  >
                    <Plus size={20} style={{ marginRight: theme.spacing(1) }} />
                    Create an Agent
                  </MenuItem>
                  {agents.agents.map((agent) => (
                    <MenuItem
                      key={agent.name}
                      onClick={() => setAgentAnchorEl(null)}
                      {...asLink(
                        generatePath(
                          absoluteRouteMap.children.org.children.projects
                            .children.agents.path,
                          { orgId, projectId, agentId: agent.name },
                        ),
                      )}
                    >
                      <Stack direction="row" gap={1} alignItems="center">
                        {agent.displayName}
                        {agent.provisioning.type === "external" && (
                          <Chip
                            label={"External"}
                            size="small"
                            variant="outlined"
                          />
                        )}
                      </Stack>
                    </MenuItem>
                  ))}
                </Menu>
              </>
            )}
          </>
        )}
      </Header.Switchers>
    </>
  );
}
