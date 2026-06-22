// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type InfraResourceManager interface {
	ListOrgEnvironments(ctx context.Context, orgName string) ([]*models.EnvironmentResponse, error)
	GetProjectDeploymentPipeline(ctx context.Context, orgName string, projectName string) (*models.DeploymentPipelineResponse, error)
	CreateOrgDeploymentPipeline(ctx context.Context, orgName string, displayName string, description *string, projectName *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error)
	UpdateOrgDeploymentPipeline(ctx context.Context, orgName string, pipelineName string, displayName *string, description *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error)
	ListOrganizations(ctx context.Context, limit int, offset int) ([]*models.OrganizationResponse, int32, error)
	GetOrganization(ctx context.Context, orgName string) (*models.OrganizationResponse, error)
	ListProjects(ctx context.Context, orgName string, limit int, offset int) ([]*models.ProjectResponse, int32, error)
	GetProject(ctx context.Context, orgName string, projectName string) (*models.ProjectResponse, error)
	CreateProject(ctx context.Context, orgName string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error)
	UpdateProject(ctx context.Context, orgName string, projectName string, payload spec.UpdateProjectRequest) (*models.ProjectResponse, error)
	DeleteProject(ctx context.Context, orgName string, projectName string) error
	DeleteOrgDeploymentPipeline(ctx context.Context, orgName, pipelineName string) error
	ListOrgDeploymentPipelines(ctx context.Context, orgName string, limit int, offset int) ([]*models.DeploymentPipelineResponse, int, error)
	GetDataplanes(ctx context.Context, orgName string) ([]*models.DataPlaneResponse, error)
}

type infraResourceManager struct {
	ocClient client.OpenChoreoClient
	logger   *slog.Logger
}

func NewInfraResourceManager(
	openChoreoClient client.OpenChoreoClient,
	logger *slog.Logger,
) InfraResourceManager {
	return &infraResourceManager{
		ocClient: openChoreoClient,
		logger:   logger,
	}
}

func (s *infraResourceManager) ListOrganizations(_ context.Context, _ int, _ int) ([]*models.OrganizationResponse, int32, error) {
	return []*models.OrganizationResponse{{
		Name:        "default",
		DisplayName: "default",
		Namespace:   "default",
	}}, 1, nil
}

func (s *infraResourceManager) GetOrganization(ctx context.Context, orgName string) (*models.OrganizationResponse, error) {
	s.logger.Debug("GetOrganization called", "orgName", orgName)

	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization from OpenChoreo", "orgName", orgName, "error", err)
		return nil, err
	}

	s.logger.Info("Fetched organization successfully", "orgName", orgName)
	return org, nil
}

func (s *infraResourceManager) CreateProject(ctx context.Context, orgName string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error) {
	s.logger.Debug("CreateProject called", "orgName", orgName, "projectName", payload.Name, "deploymentPipeline", payload.DeploymentPipeline)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization", "orgName", orgName, "error", err)
		return nil, err
	}

	// Create project in OpenChoreo
	req := client.CreateProjectRequest{
		Name:               payload.Name,
		DisplayName:        payload.DisplayName,
		Description:        utils.StrPointerAsStr(payload.Description, ""),
		DeploymentPipeline: payload.DeploymentPipeline,
	}

	if err := s.ocClient.CreateProject(ctx, orgName, req); err != nil {
		s.logger.Error("Failed to create project in OpenChoreo", "orgName", orgName, "projectName", payload.Name, "error", err)
		return nil, err
	}
	s.logger.Info("Project created successfully", "orgName", orgName, "projectName", payload.Name)

	return &models.ProjectResponse{
		Name:               payload.Name,
		OrgName:            orgName,
		DisplayName:        payload.DisplayName,
		Description:        utils.StrPointerAsStr(payload.Description, ""),
		CreatedAt:          time.Now(),
		DeploymentPipeline: payload.DeploymentPipeline,
	}, nil
}

func (s *infraResourceManager) UpdateProject(ctx context.Context, orgName string, projectName string, payload spec.UpdateProjectRequest) (*models.ProjectResponse, error) {
	s.logger.Info("Updating project", "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization", "orgName", orgName, "error", err)
		return nil, err
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}
	// Todo: verify existence of deployment pipeline if deployment pipeline is being updated

	// Update project in OpenChoreo using PatchProject
	patchReq := client.PatchProjectRequest{
		DisplayName:        payload.DisplayName,
		Description:        payload.Description,
		DeploymentPipeline: payload.DeploymentPipeline,
	}
	if err := s.ocClient.PatchProject(ctx, orgName, projectName, patchReq); err != nil {
		s.logger.Error("Failed to update project in OpenChoreo", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to update project: %w", err)
	}

	// Fetch updated project
	updatedProject, err := s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to fetch updated project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}

	s.logger.Info("Project updated successfully", "orgName", orgName, "projectName", projectName)

	return updatedProject, nil
}

func (s *infraResourceManager) ListProjects(ctx context.Context, orgName string, limit int, offset int) ([]*models.ProjectResponse, int32, error) {
	s.logger.Debug("ListProjects called", "orgName", orgName, "limit", limit, "offset", offset)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization", "orgName", orgName, "error", err)
		return nil, 0, err
	}

	projects, err := s.ocClient.ListProjects(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to list projects", "orgName", orgName, "error", err)
		return nil, 0, fmt.Errorf("failed to list projects for organization %s: %w", orgName, err)
	}
	s.logger.Debug("Retrieved projects", "orgName", orgName, "totalCount", len(projects))

	total := len(projects)
	// Apply pagination
	start := offset
	if start > len(projects) {
		start = len(projects)
	}
	end := offset + limit
	if end > len(projects) {
		end = len(projects)
	}
	paginatedProjects := projects[start:end]

	s.logger.Info("Fetched projects successfully", "orgName", orgName, "count", len(paginatedProjects), "total", total)
	return paginatedProjects, int32(total), nil
}

func (s *infraResourceManager) DeleteProject(ctx context.Context, orgName string, projectName string) error {
	s.logger.Debug("DeleteProject called", "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization", "orgName", orgName, "error", err)
		return err
	}
	// Check agents exist for the project
	s.logger.Debug("Checking for associated agents", "projectName", projectName)
	agents, err := s.ocClient.ListComponents(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			s.logger.Warn("Project not found while listing components; delete is idempotent", "orgName", orgName, "projectName", projectName)
			return nil
		}
		s.logger.Error("Failed to list agents for project", "projectName", projectName, "error", err)
		return err
	}
	if len(agents) > 0 {
		s.logger.Warn("Cannot delete project with associated agents", "orgName", orgName, "projectName", projectName, "agentCount", len(agents))
		return utils.ErrProjectHasAssociatedAgents
	}
	s.logger.Debug("No associated agents found, proceeding with deletion", "projectName", projectName)

	// Delete project from OpenChoreo
	err = s.ocClient.DeleteProject(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			s.logger.Warn("Project not found during deletion, delete is idempotent", "orgName", orgName, "projectName", projectName)
			return nil
		}
		s.logger.Error("Failed to delete project from OpenChoreo", "orgName", orgName, "projectName", projectName, "error", err)
		return err
	}
	s.logger.Info("Project deleted successfully", "orgName", orgName, "projectName", projectName)
	return nil
}

func (s *infraResourceManager) GetProject(ctx context.Context, orgName string, projectName string) (*models.ProjectResponse, error) {
	s.logger.Debug("GetProject called", "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization", "orgName", orgName, "error", err)
		return nil, err
	}

	project, err := s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get project from OpenChoreo", "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	s.logger.Info("Fetched project successfully", "orgName", orgName, "projectName", projectName)
	return project, nil
}

func (s *infraResourceManager) ListOrgDeploymentPipelines(ctx context.Context, orgName string, limit int, offset int) ([]*models.DeploymentPipelineResponse, int, error) {
	s.logger.Debug("ListOrgDeploymentPipelines called", "orgName", orgName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization", "orgName", orgName, "error", err)
		return nil, 0, err
	}

	s.logger.Debug("Fetching deployment pipelines from OpenChoreo", "orgName", orgName)
	deploymentPipelines, err := s.ocClient.ListDeploymentPipelines(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get deployment pipelines from OpenChoreo", "orgName", orgName, "error", err)
		return nil, 0, fmt.Errorf("failed to get deployment pipelines for organization %s: %w", orgName, err)
	}

	s.logger.Info("Fetched deployment pipelines successfully", "orgName", orgName, "count", len(deploymentPipelines))
	total := len(deploymentPipelines)
	// Apply pagination
	start := offset
	if start > len(deploymentPipelines) {
		start = len(deploymentPipelines)
	}
	end := offset + limit
	if end > len(deploymentPipelines) {
		end = len(deploymentPipelines)
	}
	paginatedDeploymentPipelines := deploymentPipelines[start:end]

	return paginatedDeploymentPipelines, total, nil
}

func (s *infraResourceManager) ListOrgEnvironments(ctx context.Context, orgName string) ([]*models.EnvironmentResponse, error) {
	s.logger.Debug("ListOrgEnvironments called", "orgName", orgName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization from OpenChoreo", "orgName", orgName, "error", err)
		return nil, err
	}
	s.logger.Debug("Fetching environments from OpenChoreo", "orgName", orgName)
	environments, err := s.ocClient.ListEnvironments(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get environments from OpenChoreo", "orgName", orgName, "error", err)
		return nil, err
	}

	s.logger.Info("Fetched environments successfully", "orgName", orgName, "count", len(environments))
	return environments, nil
}

func (s *infraResourceManager) GetProjectDeploymentPipeline(ctx context.Context, orgName string, projectName string) (*models.DeploymentPipelineResponse, error) {
	s.logger.Debug("GetProjectDeploymentPipeline called", "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization", "orgName", orgName, "error", err)
		return nil, err
	}

	s.logger.Debug("Fetching deployment pipeline from OpenChoreo", "orgName", orgName, "projectName", projectName)
	deploymentPipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get deployment pipeline from OpenChoreo", "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	s.logger.Info("Fetched deployment pipeline successfully", "orgName", orgName, "projectName", projectName)

	return deploymentPipeline, nil
}

func (s *infraResourceManager) CreateOrgDeploymentPipeline(ctx context.Context, orgName string, displayName string, description *string, projectName *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error) {
	s.logger.Info("Creating deployment pipeline", "orgName", orgName, "displayName", displayName)

	pipelineName := slugify(displayName) // slugify is defined in evaluator_manager.go
	if pipelineName == "" {
		return nil, fmt.Errorf("invalid display name: cannot derive a valid pipeline name")
	}

	created, err := s.ocClient.CreateDeploymentPipeline(ctx, orgName, pipelineName, &displayName, description, promotionPaths)
	if err != nil {
		s.logger.Error("Failed to create deployment pipeline", "orgName", orgName, "error", err)
		return nil, err
	}

	// If a projectName was provided, link the newly created pipeline as the project's deploymentPipelineRef.
	// OpenChoreo's DeploymentPipeline model has no projectName; the project↔pipeline link is represented
	// via Project.spec.deploymentPipelineRef and must be set separately.
	if projectName != nil && *projectName != "" {
		project, getErr := s.ocClient.GetProject(ctx, orgName, *projectName)
		if getErr != nil {
			s.logger.Error("Failed to fetch project for pipeline linkage", "orgName", orgName, "projectName", *projectName, "error", getErr)
			return nil, fmt.Errorf("failed to link deployment pipeline to project: %w", getErr)
		}
		if patchErr := s.ocClient.PatchProject(ctx, orgName, *projectName, client.PatchProjectRequest{
			DisplayName:        project.DisplayName,
			Description:        project.Description,
			DeploymentPipeline: pipelineName,
		}); patchErr != nil {
			s.logger.Error("Failed to patch project with deployment pipeline ref", "orgName", orgName, "projectName", *projectName, "pipelineName", pipelineName, "error", patchErr)
			return nil, fmt.Errorf("failed to link deployment pipeline to project: %w", patchErr)
		}
	}

	s.logger.Info("Deployment pipeline created successfully", "orgName", orgName, "pipelineName", pipelineName)
	return created, nil
}

func (s *infraResourceManager) UpdateOrgDeploymentPipeline(ctx context.Context, orgName string, pipelineName string, displayName *string, description *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error) {
	s.logger.Info("Updating deployment pipeline", "orgName", orgName, "pipelineName", pipelineName)
	updated, err := s.ocClient.UpdateDeploymentPipeline(ctx, orgName, pipelineName, displayName, description, promotionPaths)
	if err != nil {
		s.logger.Error("Failed to update deployment pipeline", "orgName", orgName, "pipelineName", pipelineName, "error", err)
		return nil, err
	}
	s.logger.Info("Deployment pipeline updated successfully", "orgName", orgName, "pipelineName", pipelineName)
	return updated, nil
}

func (s *infraResourceManager) DeleteOrgDeploymentPipeline(ctx context.Context, orgName string, pipelineName string) error {
	s.logger.Info("Deleting deployment pipeline", "orgName", orgName, "pipelineName", pipelineName)

	// Block deletion if any project still references this deployment pipeline.
	s.logger.Debug("Checking for projects referencing the deployment pipeline", "orgName", orgName, "pipelineName", pipelineName)
	projects, err := s.ocClient.ListProjects(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to list projects while checking deployment pipeline references", "orgName", orgName, "pipelineName", pipelineName, "error", err)
		return fmt.Errorf("failed to verify deployment pipeline references: %w", err)
	}
	var referencingProjects []string
	for _, project := range projects {
		if project != nil && project.DeploymentPipeline == pipelineName {
			referencingProjects = append(referencingProjects, project.Name)
		}
	}
	if len(referencingProjects) > 0 {
		s.logger.Warn("Cannot delete deployment pipeline referenced by projects", "orgName", orgName, "pipelineName", pipelineName, "projects", referencingProjects)
		return fmt.Errorf("%w: %v", utils.ErrDeploymentPipelineInUse, referencingProjects)
	}

	if err := s.ocClient.DeleteOrgDeploymentPipeline(ctx, orgName, pipelineName); err != nil {
		s.logger.Error("Failed to delete deployment pipeline", "orgName", orgName, "pipelineName", pipelineName, "error", err)
		return fmt.Errorf("failed to delete deployment pipeline: %w", err)
	}

	s.logger.Info("Deployment pipeline deleted successfully", "orgName", orgName, "pipelineName", pipelineName)
	return nil
}

func (s *infraResourceManager) GetDataplanes(ctx context.Context, orgName string) ([]*models.DataPlaneResponse, error) {
	s.logger.Debug("GetDataplanes called", "orgName", orgName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization", "orgName", orgName, "error", err)
		return nil, err
	}

	s.logger.Debug("Fetching dataplanes from OpenChoreo")
	dataplanes, err := s.ocClient.ListDataPlanes(ctx)
	if err != nil {
		s.logger.Error("Failed to get dataplanes from OpenChoreo", "error", err)
		return nil, err
	}

	s.logger.Info("Fetched dataplanes successfully", "count", len(dataplanes))
	return dataplanes, nil
}
