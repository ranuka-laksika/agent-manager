// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

// UNIT tests for monitorManagerService.DeleteMonitorsByAgent — the agent-deletion
// cleanup path (issue #928). No build tag, so this runs in the fast unit tier with
// every collaborator mocked. fakeProvisioner and discardLogger already exist in this
// package and are reused here.
package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func TestDeleteMonitorsByAgent_DeletesAllMonitors(t *testing.T) {
	const ouID, projectName, agentName = "org-1", "proj-1", "agent-1"

	monitors := []models.Monitor{
		{ID: uuid.New(), Name: "monitor-a", OUID: ouID, ProjectName: projectName, AgentName: agentName},
		{ID: uuid.New(), Name: "monitor-b", OUID: ouID, ProjectName: projectName, AgentName: agentName},
	}

	var deleted []string
	monitorRepo := &repomocks.MonitorRepositoryMock{
		ListMonitorsByAgentFunc: func(gotOU, gotProj, gotAgent string) ([]models.Monitor, error) {
			if gotOU != ouID || gotProj != projectName || gotAgent != agentName {
				t.Errorf("ListMonitorsByAgent got (%s,%s,%s)", gotOU, gotProj, gotAgent)
			}
			return monitors, nil
		},
		GetMonitorByNameFunc: func(_, _, _, name string) (*models.Monitor, error) {
			for i := range monitors {
				if monitors[i].Name == name {
					return &monitors[i], nil
				}
			}
			return nil, gorm.ErrRecordNotFound
		},
		GetMonitorRunsByMonitorIDFunc: func(_ uuid.UUID) ([]models.MonitorRun, error) {
			return nil, nil
		},
		DeleteMonitorFunc: func(m *models.Monitor) error {
			deleted = append(deleted, m.Name)
			return nil
		},
	}
	mappingRepo := &repomocks.MonitorLLMMappingRepositoryMock{
		ListByMonitorIDFunc: func(_ context.Context, _ uuid.UUID) ([]models.MonitorLLMMapping, error) {
			return nil, nil
		},
		DeleteByMonitorIDFunc: func(_ context.Context, _ *gorm.DB, _ uuid.UUID) error {
			return nil
		},
	}

	s := &monitorManagerService{
		logger:                discardLogger(),
		monitorRepo:           monitorRepo,
		monitorLLMMappingRepo: mappingRepo,
		provisioner:           &fakeProvisioner{IsThunderModeFunc: func() bool { return false }},
	}

	if err := s.DeleteMonitorsByAgent(context.Background(), ouID, projectName, agentName); err != nil {
		t.Fatalf("DeleteMonitorsByAgent returned error: %v", err)
	}
	if len(deleted) != len(monitors) {
		t.Fatalf("expected %d monitors deleted, got %d (%v)", len(monitors), len(deleted), deleted)
	}
}

func TestDeleteMonitorsByAgent_NoMonitorsIsNoOp(t *testing.T) {
	monitorRepo := &repomocks.MonitorRepositoryMock{
		ListMonitorsByAgentFunc: func(_, _, _ string) ([]models.Monitor, error) {
			return nil, nil
		},
	}
	s := &monitorManagerService{logger: discardLogger(), monitorRepo: monitorRepo}

	if err := s.DeleteMonitorsByAgent(context.Background(), "org", "proj", "agent"); err != nil {
		t.Fatalf("expected nil error for no monitors, got %v", err)
	}
	if len(monitorRepo.DeleteMonitorCalls()) != 0 {
		t.Fatalf("expected no DeleteMonitor calls, got %d", len(monitorRepo.DeleteMonitorCalls()))
	}
}

func TestDeleteMonitorsByAgent_ListErrorIsReturned(t *testing.T) {
	sentinel := errors.New("db down")
	monitorRepo := &repomocks.MonitorRepositoryMock{
		ListMonitorsByAgentFunc: func(_, _, _ string) ([]models.Monitor, error) {
			return nil, sentinel
		},
	}
	s := &monitorManagerService{logger: discardLogger(), monitorRepo: monitorRepo}

	err := s.DeleteMonitorsByAgent(context.Background(), "org", "proj", "agent")
	if err == nil {
		t.Fatal("expected error when listing monitors fails, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
}
