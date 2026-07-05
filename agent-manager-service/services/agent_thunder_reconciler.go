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
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

const (
	reconcilerTickInterval = 1 * time.Minute
	// reconcilerLockID is a distinct PostgreSQL advisory lock ID from
	// schedulerLockID (monitor_scheduler.go) — each background loop in this
	// service needs its own ID so they don't block each other.
	reconcilerLockID    = int64(739281457)
	reconcilerBatchSize = 50
)

// AgentThunderReconcilerService sweeps PENDING agent Thunder bindings whose
// retry time has arrived and retries them, up to the max-attempts budget
// (Section 6 of the AgentID architecture doc).
type AgentThunderReconcilerService interface {
	Start(ctx context.Context) error
	Stop() error
}

type agentThunderReconcilerService struct {
	provisioning AgentThunderProvisioningService
	repo         repositories.AgentThunderClientRepository
	logger       *slog.Logger
	stopCh       chan struct{}
	stopOnce     sync.Once
}

// NewAgentThunderReconcilerService creates a new AgentThunderReconcilerService.
func NewAgentThunderReconcilerService(
	provisioning AgentThunderProvisioningService,
	repo repositories.AgentThunderClientRepository,
	logger *slog.Logger,
) AgentThunderReconcilerService {
	return &agentThunderReconcilerService{
		provisioning: provisioning,
		repo:         repo,
		logger:       logger,
		stopCh:       make(chan struct{}),
	}
}

func (s *agentThunderReconcilerService) Start(ctx context.Context) error {
	s.logger.Info("Initializing agent thunder provisioning reconciler")
	go s.runLoop(ctx)
	s.logger.Info("Agent thunder provisioning reconciler started")
	return nil
}

func (s *agentThunderReconcilerService) Stop() error {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.logger.Info("Agent thunder provisioning reconciler stopped")
	})
	return nil
}

func (s *agentThunderReconcilerService) runLoop(ctx context.Context) {
	ticker := time.NewTicker(reconcilerTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runCycle(ctx)
		case <-s.stopCh:
			s.logger.Info("Agent thunder reconciler loop stopped")
			return
		case <-ctx.Done():
			s.logger.Info("Agent thunder reconciler loop context cancelled")
			return
		}
	}
}

// runCycle sweeps due bindings under a PostgreSQL advisory lock so only one
// replica of this service retries a given binding at a time — mirrors
// monitor_scheduler.go's runSchedulerCycle exactly.
func (s *agentThunderReconcilerService) runCycle(ctx context.Context) {
	tx := db.GetDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		s.logger.Error("Failed to begin transaction for agent thunder reconciler advisory lock", "error", tx.Error)
		return
	}

	var locked bool
	if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", reconcilerLockID).Scan(&locked).Error; err != nil {
		s.logger.Error("Failed to try agent thunder reconciler advisory lock", "error", err)
		tx.Rollback()
		return
	}
	if !locked {
		s.logger.Debug("Another instance is running the agent thunder reconciler, skipping cycle")
		tx.Rollback()
		return
	}

	due, err := s.repo.FindDue(ctx, time.Now(), reconcilerBatchSize)
	if err != nil {
		s.logger.Error("Failed to query due agent thunder bindings", "error", err)
		tx.Rollback()
		return
	}

	// Release the lock/connection now — ClaimForAttempt already guards each
	// binding individually, so the lock only needs to cover the FindDue scan,
	// not the slow Thunder/OpenBao calls below.
	if err := tx.Commit().Error; err != nil {
		s.logger.Error("Failed to commit agent thunder reconciler advisory lock transaction", "error", err)
		return
	}

	if len(due) > 0 {
		s.logger.Info("Retrying due agent thunder bindings", "count", len(due))
	}
	for _, binding := range due {
		s.provisioning.AttemptProvision(ctx, binding)
	}
}
