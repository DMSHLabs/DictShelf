// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"runtime"
	"sync/atomic"

	"github.com/DMSHLabs/DictShelf/internal/indexconst"
	"github.com/DMSHLabs/DictShelf/internal/logx"
)

type ingestParallelismPolicy struct {
	power       string
	activeProcs int
	demandHolds int
}

type ingestParallelismSnapshot struct {
	logicalCPUs, gomaxprocs                         int
	power, powerLimitReason, workerLimitReason      string
	activeProcs, demandHolds, dictionaryLimit       int
	activeDictionaries, workersCreated, workerLimit int
	workersRunning, inlinePreparing                 int
	queuedBatches, inFlightBatches, readyBatches    int
	outstandingBatches                              int
	usedBytes, memoryBudget                         int64
}

var ingestParallelismDiagnostics atomic.Bool
var ingestParallelismPolicyState atomic.Pointer[ingestParallelismPolicy]

// SetIngestParallelismDiagnostics gates indexing stages and ingest-pipeline snapshots.
// The surrounding system log must also be enabled to retain the records.
func SetIngestParallelismDiagnostics(enabled bool) {
	ingestParallelismDiagnostics.Store(enabled)
}

// SetIngestParallelismPolicy records the server's current power-managed
// processor policy without making the store depend on the server package.
func SetIngestParallelismPolicy(power string, activeProcs, demandHolds int) {
	ingestParallelismPolicyState.Store(&ingestParallelismPolicy{
		power: power, activeProcs: activeProcs, demandHolds: demandHolds,
	})
}

func logIngestParallelism(event, dictionary, job string) {
	if !ingestParallelismDiagnostics.Load() {
		return
	}
	s := sharedIngestPool.snapshot()
	logx.SystemDiagnostic("ingest_parallelism event=%s job=%s dictionary=%q mode=%s logical_cpus=%d gomaxprocs=%d power=%s active_procs=%d demand_holds=%d power_limit_reason=%s dictionary_limit=%d active_dictionaries=%d workers_created=%d worker_limit=%d worker_limit_reason=%s workers_running=%d inline_preparing=%d queued_batches=%d in_flight_batches=%d ready_batches=%d outstanding_batches=%d memory_used_bytes=%d memory_budget_bytes=%d",
		event, job, dictionary, indexconst.ModeName(), s.logicalCPUs, s.gomaxprocs, s.power, s.activeProcs, s.demandHolds, s.powerLimitReason,
		s.dictionaryLimit, s.activeDictionaries, s.workersCreated, s.workerLimit, s.workerLimitReason, s.workersRunning, s.inlinePreparing,
		s.queuedBatches, s.inFlightBatches, s.readyBatches, s.outstandingBatches, s.usedBytes, s.memoryBudget)
}

func (p *ingestPool) snapshot() ingestParallelismSnapshot {
	logical, gomax := runtime.NumCPU(), runtime.GOMAXPROCS(0)
	policy := ingestParallelismPolicyState.Load()
	s := ingestParallelismSnapshot{
		logicalCPUs: logical, gomaxprocs: gomax,
		dictionaryLimit: indexconst.IndexDictionaryLimitForMode(logical),
		workersCreated:  int(p.workers.Load()), workersRunning: int(p.running.Load()),
		inlinePreparing: int(p.inlineRunning.Load()), usedBytes: p.usedBytes.Load(), memoryBudget: p.memCapacity.Load(),
		power: "active", powerLimitReason: "runtime_default",
	}
	if policy != nil {
		s.power, s.activeProcs, s.demandHolds = policy.power, policy.activeProcs, policy.demandHolds
	}
	if s.activeProcs > 0 {
		s.powerLimitReason = "active_power_limit"
		if s.power != "active" && s.demandHolds == 0 {
			s.powerLimitReason = s.power + "_power_cap"
		} else if s.power != "active" && s.demandHolds > 0 {
			s.powerLimitReason = "user_waited_work_holds_active_limit"
		}
	}
	s.workerLimit = p.workerLimit(0)
	if sessions := p.sessions.Load(); sessions != nil {
		s.activeDictionaries = len(*sessions)
		for _, session := range *sessions {
			session.mu.Lock()
			s.queuedBatches += len(session.queue)
			s.inFlightBatches += session.inFlight
			s.outstandingBatches += session.outstanding
			session.mu.Unlock()
			s.readyBatches += len(session.results)
		}
		s.workerLimit = p.workerLimit(s.activeDictionaries)
	}
	s.workerLimitReason = "worker_pool_available"
	if indexconst.CurrentMode() == indexconst.ModeParallelDictionariesSingleWorker {
		s.workerLimitReason = "benchmark_single_worker_per_dictionary"
	} else if gomax < logical {
		s.workerLimitReason = "gomaxprocs_runtime_cap"
	} else if s.workerLimit == 0 {
		s.workerLimitReason = "reader_writer_and_ui_processors_reserved"
	} else if s.activeDictionaries > 0 && s.workerLimit < s.workersCreated {
		s.workerLimitReason = "reader_writer_and_ui_processors_reserved"
	} else if s.workerLimit < indexconst.IngestWorkerLimitForMode(logical, max(1, s.activeDictionaries)) {
		s.workerLimitReason = "worker_pool_size_cap"
	}
	return s
}
