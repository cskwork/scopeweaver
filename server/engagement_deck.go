package server

// Deck generation: shell out to the vendored Sibermate deck builder.
//
// The script is embedded in the binary rather than shipped as a loose file: a
// report generator that silently disappears because someone moved a directory
// is worse than one that fails loudly, and embedding makes the pipeline
// identical on every host.

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed deckgen/make_deck.py
var deckScript []byte

// GenerateDeck renders the Sibermate-format PPTX for a collected run.
func (c *EngagementCollector) GenerateDeck(ctx context.Context, runID int64, outDir string) (string, error) {
	py, err := pythonWithPptx(ctx)
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp("", "make_deck_*.py")
	if err != nil {
		return "", err
	}
	scriptPath := tmp.Name()
	defer os.Remove(scriptPath)
	if _, err := tmp.Write(deckScript); err != nil {
		tmp.Close()
		return "", err
	}
	tmp.Close()

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(outDir, fmt.Sprintf("deck-run-%d.pptx", runID))
	cmd := exec.CommandContext(ctx, py, scriptPath, fmt.Sprint(runID), out)
	cmd.Env = append(os.Environ(), "ARTEX_PG_DSN="+c.dsnForScript())
	combined, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("make_deck: %v: %s", err, strings.TrimSpace(string(combined)))
	}
	hash, _ := fileHash(out)
	var size int64
	if st, err := os.Stat(out); err == nil {
		size = st.Size()
	}
	_, _ = c.DB.ExecContext(ctx, `
		INSERT INTO eng_log_manifest (run_id, artefact, format, path, bytes, sha256, chain_head, row_count)
		VALUES ($1,'deck_pptx','pptx',$2,$3,$4,'',0)`, runID, out, size, hash)
	return out, nil
}

// dsnForScript exposes the DSN the deck script needs. The collector's DB is
// already connected, but psql in the child process needs the string itself.
func (c *EngagementCollector) dsnForScript() string {
	if c.DSN != "" {
		return c.DSN
	}
	return os.Getenv("ARTEX_PG_DSN")
}

// pythonWithPptx returns an interpreter that can import python-pptx.
//
// The service runs with a fixed PATH whose python3 is the system interpreter,
// while an operator shell often resolves a venv that already has the package.
// Probing instead of assuming makes the deck render identically under systemd
// and interactively.
func pythonWithPptx(ctx context.Context) (string, error) {
	candidates := []string{}
	if p, err := exec.LookPath("python3"); err == nil {
		candidates = append(candidates, p)
	}
	for _, p := range []string{
		"/usr/bin/python3", "/usr/local/bin/python3",
		"/home/ubuntu/.hermes/installs/*/environments/*/venv/bin/python3",
	} {
		if strings.Contains(p, "*") {
			matches, _ := filepath.Glob(p)
			candidates = append(candidates, matches...)
			continue
		}
		if _, err := os.Stat(p); err == nil {
			candidates = append(candidates, p)
		}
	}
	seen := map[string]bool{}
	var tried []string
	for _, cand := range candidates {
		if seen[cand] {
			continue
		}
		seen[cand] = true
		if err := exec.CommandContext(ctx, cand, "-c", "import pptx").Run(); err == nil {
			return cand, nil
		}
		tried = append(tried, cand)
	}
	return "", fmt.Errorf("no interpreter with python-pptx (tried %s); install with: /usr/bin/python3 -m pip install --break-system-packages python-pptx", strings.Join(tried, ", "))
}
