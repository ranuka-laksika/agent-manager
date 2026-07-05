//go:build integration

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

package services

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

func newTestReconciler(t *testing.T, provisioning AgentThunderProvisioningService) *agentThunderReconcilerService {
	t.Helper()
	repo := repositories.NewAgentThunderClientRepo(db.GetDB())
	return &agentThunderReconcilerService{
		provisioning: provisioning,
		repo:         repo,
		logger:       slog.Default(),
		stopCh:       make(chan struct{}),
	}
}

func TestAgentThunderReconciler_StartStop(t *testing.T) {
	svc := NewAgentThunderReconcilerService(nil, repositories.NewAgentThunderClientRepo(db.GetDB()), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, svc.Start(ctx))
	require.NoError(t, svc.Stop())
}

func TestAgentThunderReconciler_StopIdempotent(t *testing.T) {
	svc := NewAgentThunderReconcilerService(nil, repositories.NewAgentThunderClientRepo(db.GetDB()), slog.Default())
	require.NoError(t, svc.Stop())
	require.NoError(t, svc.Stop())
}

func TestAgentThunderReconciler_StopsOnContextCancel(t *testing.T) {
	s := newTestReconciler(t, nil)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		s.runLoop(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reconciler loop did not stop after context cancellation")
	}
}

func TestAgentThunderReconciler_StopsOnStopChannel(t *testing.T) {
	s := newTestReconciler(t, nil)

	done := make(chan struct{})
	go func() {
		s.runLoop(context.Background())
		close(done)
	}()

	close(s.stopCh)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reconciler loop did not stop after stop channel closed")
	}
}

func TestAgentThunderReconciler_AdvisoryLockBlocksConcurrentCycle(t *testing.T) {
	ctx := context.Background()

	holdTx := db.DB(ctx).Begin()
	require.NoError(t, holdTx.Error)
	defer holdTx.Rollback()

	var locked bool
	require.NoError(t, holdTx.Raw("SELECT pg_try_advisory_xact_lock(?)", reconcilerLockID).Scan(&locked).Error)
	require.True(t, locked)

	var attemptCalled atomic.Int32
	provisioning := &fakeProvisioningService{
		attemptFunc: func(_ context.Context, _ models.AgentThunderClient) { attemptCalled.Add(1) },
	}
	s := newTestReconciler(t, provisioning)

	s.runCycle(ctx)

	assert.Equal(t, int32(0), attemptCalled.Load(), "AttemptProvision must not run when another instance holds the advisory lock")
}

func TestAgentThunderReconciler_RunCycle_RetriesDueBindings(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "reconciler-cycle-agent"
	t.Cleanup(func() { _ = repo.DeleteByAgent(org, project, agent) })

	due := &models.AgentThunderClient{
		OrgName: org, ProjectName: project, AgentName: agent, EnvironmentName: "dev",
		ProvisioningType: models.AgentProvisioningTypeExternal, Status: models.AgentThunderStatusPending,
	}
	require.NoError(t, repo.Upsert(due))

	notYetDue := &models.AgentThunderClient{
		OrgName: org, ProjectName: project, AgentName: agent, EnvironmentName: "staging",
		ProvisioningType: models.AgentProvisioningTypeExternal, Status: models.AgentThunderStatusPending,
	}
	future := time.Now().Add(1 * time.Hour)
	notYetDue.NextRetryAt = &future
	require.NoError(t, repo.Upsert(notYetDue))

	var attempted []string
	provisioning := &fakeProvisioningService{
		attemptFunc: func(_ context.Context, b models.AgentThunderClient) {
			attempted = append(attempted, b.EnvironmentName)
		},
	}
	s := &agentThunderReconcilerService{provisioning: provisioning, repo: repo, logger: slog.Default(), stopCh: make(chan struct{})}

	s.runCycle(ctx)

	assert.Contains(t, attempted, "dev")
	assert.NotContains(t, attempted, "staging", "a binding whose next_retry_at is still in the future must not be retried early")
}

// fakeProvisioningService is a minimal hand-written test double for
// AgentThunderProvisioningService — only AttemptProvision is exercised by the
// reconciler, so that is the only method given a real implementation.
type fakeProvisioningService struct {
	attemptFunc func(ctx context.Context, binding models.AgentThunderClient)
}

func (f *fakeProvisioningService) ProvisionForAgent(context.Context, string, string, string, models.AgentProvisioningType, []string, string) {
}
func (f *fakeProvisioningService) ProvisionForEnvironmentIfMissing(context.Context, string, string, string, string, models.AgentProvisioningType, string) (bool, error) {
	return false, nil
}
func (f *fakeProvisioningService) AttemptProvision(ctx context.Context, binding models.AgentThunderClient) {
	f.attemptFunc(ctx, binding)
}
func (f *fakeProvisioningService) GetCredentials(context.Context, string, string, string, string) (string, string, string, error) {
	return "", "", "", nil
}
func (f *fakeProvisioningService) RegenerateSecret(context.Context, string, string, string, string) (models.AgentProvisioningType, string, string, error) {
	return "", "", "", nil
}
func (f *fakeProvisioningService) RevokeSecret(context.Context, string, string, string, string) (string, error) {
	return "", nil
}
func (f *fakeProvisioningService) DeleteAllBindings(context.Context, string, string, string) {}
func (f *fakeProvisioningService) GetIdentityViews(context.Context, string, string, string) ([]models.AgentIdentityEnvironmentView, error) {
	return nil, nil
}

// compile-time interface check
var _ AgentThunderProvisioningService = (*fakeProvisioningService)(nil)
