package server

// Automation entry points: wire the engagement logging + report pipeline so it
// runs on every task without any caller remembering to invoke it.
//
// The operator's requirement is "every time ScopeWeaver runs there must be
// logs". That is satisfied by hooking the single lifecycle choke point
// (SetTaskStatus -> terminal) rather than asking each call site to collect.
//
// Failure policy: collection or rendering failing must never take the engine
// down or fail the task. It is logged and the run is simply left without
// artefacts, which is why the report itself states coverage.

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// EnableEngagementAutomation installs the terminal-task hook.
//
// outputRoot receives one directory per task: <outputRoot>/task-<id>/ containing
// the five log exports, the tech report (html/pdf/docx) and the deck (pptx).
// An empty outputRoot defaults to <dataDir>/engagement-reports.
func (m *Manager) EnableEngagementAutomation(outputRoot string) {
	if outputRoot == "" {
		outputRoot = filepath.Join(m.dir, "engagement-reports")
	}
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		log.Printf("[engagement] cannot create report root %s: %v", outputRoot, err)
	}
	m.engagementHook = func(taskID int64, status string) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[engagement] panic while collecting task %d: %v", taskID, r)
			}
		}()
		start := time.Now()
		outDir := filepath.Join(outputRoot, fmt.Sprintf("task-%d", taskID))
		c := &EngagementCollector{
			DB:          m.pg.DB,
			TemplateRev: EngagementTemplateRev,
			EvidenceDir: filepath.Join(outDir, "evidence"),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		runID, err := c.CollectEngagementLogs(ctx, taskID)
		if err != nil {
			log.Printf("[engagement] task %d (%s): collect failed: %v", taskID, status, err)
			return
		}
		produced, warnings, err := c.GenerateAll(ctx, runID, outDir)
		if err != nil {
			log.Printf("[engagement] task %d run %d: report failed: %v", taskID, runID, err)
			return
		}
		for _, w := range warnings {
			log.Printf("[engagement] task %d run %d: %s", taskID, runID, w)
		}
		// Deck is best-effort: a PPTX failure must not lose the tech report.
		if deck, err := c.GenerateDeck(ctx, runID, outDir); err != nil {
			log.Printf("[engagement] task %d run %d: deck skipped: %v", taskID, runID, err)
		} else {
			produced["pptx"] = deck
		}
		_ = status
		log.Printf("[engagement] task %d run %d collected+rendered in %s (%d formats)",
			taskID, runID, time.Since(start).Round(time.Millisecond), len(produced))
	}
}

// EngagementTemplateRev identifies the static report/deck layout. Bump it only
// when the layout changes, never per run: a report states which revision
// produced it so two documents from different builds are distinguishable.
const EngagementTemplateRev = "sibermate-2026.11"
