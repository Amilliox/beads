//go:build regression

// Oracle A — conformance harness for basic bd CLI scenarios.
//
// Builds both binaries (candidate = current worktree, baseline = ref),
// runs each command on both, and compares exit code + normalized output.
// Each scenario starts with a fresh workspace (tmpfs-backed temp dir).
//
// Run:  go test -tags=regression -run TestOracleA -count=1 -v ./tests/regression/
// Or:   make oracle-a
package regression

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Oracle types
// ---------------------------------------------------------------------------

type cmdResult struct {
	exitCode int
	stdout   string
	stderr   string
}

type oracleResult struct {
	name    string
	passed  bool
	bResult cmdResult
	cResult cmdResult
	bNorm   string
	cNorm   string
	failMsg string
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

// runOracleCmd executes bd and returns (exitCode, stdout, stderr) separately.
func runOracleCmd(bdPath, workDir string, args []string) cmdResult {
	cmd := exec.Command(bdPath, args...)
	cmd.Dir = workDir

	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + workDir,
		"BEADS_TEST_MODE=1",
		"BD_NO_DAEMON=1",
		"BEADS_NO_DAEMON=1",
		"BD_DISABLE_METRICS=1",
		"BD_DISABLE_EVENT_FLUSH=1",
		"GIT_CONFIG_NOSYSTEM=1",
		"BD_NO_CLAUDE=1",
		"BD_NO_COPILOT=1",
		"BEADS_NO_INSTALL_INTEGRATIONS=1",
	}
	if testDoltServerPort != 0 {
		portStr := fmt.Sprintf("%d", testDoltServerPort)
		env = append(env,
			"BEADS_DOLT_PORT="+portStr,
			"BEADS_DOLT_SERVER_PORT="+portStr,
		)
	} else if v := os.Getenv("BEADS_DOLT_PORT"); v != "" {
		// Fallback: forward BEADS_DOLT_PORT from parent env when Docker is unavailable
		env = append(env,
			"BEADS_DOLT_PORT="+v,
			"BEADS_DOLT_SERVER_PORT="+v,
		)
	}
	if v := os.Getenv("TMPDIR"); v != "" {
		env = append(env, "TMPDIR="+v)
	}
	cmd.Env = env

	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return cmdResult{
		exitCode: exitCode,
		stdout:   stdoutBuf.String(),
		stderr:   stderrBuf.String(),
	}
}

// defaultNormalizer replaces volatile output with placeholders.
func defaultNormalizer(out string) string {
	s := out

	// STEP 1: Strip integration install messages (candidate-only extra output)
	// These are placed by bd init in newer versions
	integrationRe := regexp.MustCompile(`(?m)^.*(Claude hooks|Claude Code|Beads agent|Codex native|Codex instructions|No additional|Restart |Installing |✓ Registered|✓ Created new).*$`)
	s = integrationRe.ReplaceAllString(s, "")
	integrationRe2 := regexp.MustCompile(`(?m)^.*(File: |Settings: |Skill: ).*$`)
	s = integrationRe2.ReplaceAllString(s, "")

	// Strip tip lines
	tipRe := regexp.MustCompile(`(?m)^.*Tip: Install.*$`)
	s = tipRe.ReplaceAllString(s, "")

	// Strip DESCRIPTION (none) section
	descRe := regexp.MustCompile(`(?m)(^DESCRIPTION$\n?\s*\(none\)$\n?)`)
	s = descRe.ReplaceAllString(s, "")

	// STEP 2: Normalize format differences (before ID normalization so original IDs are still present)

	// Normalize "✓ Updated issue: <id> — title" -> "✓ Updated issue: <id>"
	updatedRe := regexp.MustCompile(`(✓ Updated issue: [a-zA-Z0-9]+-[a-zA-Z0-9]+) — .+`)
	s = updatedRe.ReplaceAllString(s, "$1")

	// Normalize "✓ Closed <id> — title:" -> "✓ Closed <id>:"
	closedRe := regexp.MustCompile(`(✓ Closed [a-zA-Z0-9]+-[a-zA-Z0-9]+) — .+?: `)
	s = closedRe.ReplaceAllString(s, "$1: ")

	// Normalize dep_add labels: "tXX (child) depends on tYY (parent)" -> "tXX depends on tYY"
	depRe := regexp.MustCompile(`([a-zA-Z0-9]+-[a-zA-Z0-9]+) \([^)]+\) depends on ([a-zA-Z0-9]+-[a-zA-Z0-9]+) \([^)]+\)`)
	s = depRe.ReplaceAllString(s, "$1 depends on $2")

	// STEP 3: Replace volatile content with placeholders
	// ORDER MATTERS: dates first, then IDs (dates contain dashes too)

	// Replace RFC3339 timestamps -> <DATE>
	tsRe := regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?`)
	s = tsRe.ReplaceAllString(s, "<DATE>")

	// Replace date-only: YYYY-MM-DD -> <DATE>
	dateRe := regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	s = dateRe.ReplaceAllString(s, "<DATE>")

	// Replace UUIDs (before IDs, UUIDs also have dashes)
	uuidRe := regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)
	s = uuidRe.ReplaceAllString(s, "<UUID>")

	// Replace issue IDs: bd-xxx or tXXX-XXX -> <ID>
	idRe := regexp.MustCompile(`[a-zA-Z0-9]+-[a-zA-Z0-9]+`)
	s = idRe.ReplaceAllString(s, "<ID>")

	// Collapse repeated newlines
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")

	// Normalize whitespace: collapse multiple spaces, trim
	s = regexp.MustCompile(`[ \t]+`).ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	return s
}

// idOutputNormalizer normalizes output that contains an ID (create --silent).
func idOutputNormalizer(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed != "" {
		return "<ID>"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Raw workspace (no bd init)
// ---------------------------------------------------------------------------

// oracleRawWorkspace creates a minimal git repo without running bd init.
func oracleRawWorkspace(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "bd-oracle-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	git := exec.Command("git", "init")
	git.Dir = dir
	if out, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	for _, cfg := range []string{"user.name", "user.email"} {
		val := "oracle"
		if cfg == "user.email" {
			val = "oracle@test"
		}
		c := exec.Command("git", "config", cfg, val)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git config %s: %v\n%s", cfg, err, out)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, ".gitkeep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ac := exec.Command("git", "add", ".")
	ac.Dir = dir
	if out, err := ac.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	cm := exec.Command("git", "commit", "-m", "initial", "--allow-empty")
	cm.Dir = dir
	if out, err := cm.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	return dir
}

// ---------------------------------------------------------------------------
// Scenario runner
// ---------------------------------------------------------------------------

// createInWS runs "bd create --silent" with given args and returns the ID.
func createInWS(t *testing.T, bdPath, workDir string, args []string) string {
	t.Helper()
	allArgs := append([]string{"create", "--silent"}, args...)
	res := runOracleCmd(bdPath, workDir, allArgs)
	if res.exitCode != 0 {
		t.Fatalf("%s create %v failed (exit %d): %s",
			bdPath, args, res.exitCode, res.stderr)
	}
	return strings.TrimSpace(res.stdout)
}

// runAndCompare runs args on both binaries and compares exit code + output.
func runAndCompare(t *testing.T, name string, args []string,
	bDir, cDir string, norm func(string) string) oracleResult {

	t.Helper()
	if norm == nil {
		norm = defaultNormalizer
	}

	bRes := runOracleCmd(baselineBin, bDir, args)
	cRes := runOracleCmd(candidateBin, cDir, args)

	bNorm := norm(bRes.stdout + bRes.stderr)
	cNorm := norm(cRes.stdout + cRes.stderr)

	passed := (bRes.exitCode == cRes.exitCode) && (bNorm == cNorm)
	r := oracleResult{
		name:    name,
		passed:  passed,
		bResult: bRes,
		cResult: cRes,
		bNorm:   bNorm,
		cNorm:   cNorm,
	}

	if bRes.exitCode != cRes.exitCode {
		r.failMsg = fmt.Sprintf("exit code: baseline=%d candidate=%d",
			bRes.exitCode, cRes.exitCode)
		t.Errorf("  %s: %s", name, r.failMsg)
	} else if bNorm != cNorm {
		r.failMsg = fmt.Sprintf("output mismatch")
		t.Errorf("  %s: output mismatch\n    baseline:  %q\n    candidate: %q",
			name, bNorm, cNorm)
	} else {
		t.Logf("  %s: PASS (exit=%d)", name, bRes.exitCode)
	}
	return r
}

// ---------------------------------------------------------------------------
// TestOracleA — 10 basic conformance scenarios
// ---------------------------------------------------------------------------

func TestOracleA(t *testing.T) {
	if baselineBin == "" || candidateBin == "" {
		t.Fatal("baselineBin or candidateBin not set — TestMain must run first")
	}

	var results []oracleResult

	// ── 1. bd init ───────────────────────────────────────────────────────
	t.Run("init", func(t *testing.T) {
		bDir := oracleRawWorkspace(t)
		cDir := oracleRawWorkspace(t)
		r := runAndCompare(t, "init", []string{"init", "--quiet"}, bDir, cDir, nil)
		results = append(results, r)
	})

	// ── 2. bd create "task1" -p 2 ───────────────────────────────────────
	t.Run("create", func(t *testing.T) {
		bWS := newWorkspace(t, baselineBin)
		cWS := newWorkspace(t, candidateBin)
		r := runAndCompare(t, "create",
			[]string{"create", "--silent", "task1", "-p", "2"},
			bWS.dir, cWS.dir, idOutputNormalizer)
		results = append(results, r)
	})

	// ── 3. bd show <id> ─────────────────────────────────────────────────
	t.Run("show", func(t *testing.T) {
		bWS := newWorkspace(t, baselineBin)
		cWS := newWorkspace(t, candidateBin)
		bID := createInWS(t, baselineBin, bWS.dir, []string{
			"--title", "show-me", "--type", "task", "-p", "2",
		})
		cID := createInWS(t, candidateBin, cWS.dir, []string{
			"--title", "show-me", "--type", "task", "-p", "2",
		})
		// bd show outputs the issue details
		bRes := runOracleCmd(baselineBin, bWS.dir, []string{"show", bID})
		cRes := runOracleCmd(candidateBin, cWS.dir, []string{"show", cID})
		bNorm := defaultNormalizer(bRes.stdout + bRes.stderr)
		cNorm := defaultNormalizer(cRes.stdout + cRes.stderr)
		passed := (bRes.exitCode == cRes.exitCode) && (bNorm == cNorm)
		r := oracleResult{"show", passed, bRes, cRes, bNorm, cNorm, ""}
		results = append(results, r)
		if bRes.exitCode != cRes.exitCode {
			t.Errorf("  show: exit code: baseline=%d candidate=%d",
				bRes.exitCode, cRes.exitCode)
		} else if bNorm != cNorm {
			t.Errorf("  show: output mismatch\n    baseline:  %q\n    candidate: %q",
				bNorm, cNorm)
		} else {
			t.Logf("  show: PASS (exit=%d)", bRes.exitCode)
		}
	})

	// ── 4. bd update <id> --claim ────────────────────────────────────────
	t.Run("claim", func(t *testing.T) {
		bWS := newWorkspace(t, baselineBin)
		cWS := newWorkspace(t, candidateBin)
		bID := createInWS(t, baselineBin, bWS.dir, []string{
			"--title", "claim-me", "--type", "task", "-p", "2",
		})
		cID := createInWS(t, candidateBin, cWS.dir, []string{
			"--title", "claim-me", "--type", "task", "-p", "2",
		})
		bRes := runOracleCmd(baselineBin, bWS.dir, []string{"update", bID, "--claim"})
		cRes := runOracleCmd(candidateBin, cWS.dir, []string{"update", cID, "--claim"})
		bNorm := defaultNormalizer(bRes.stdout + bRes.stderr)
		cNorm := defaultNormalizer(cRes.stdout + cRes.stderr)
		passed := (bRes.exitCode == cRes.exitCode) && (bNorm == cNorm)
		r := oracleResult{"claim", passed, bRes, cRes, bNorm, cNorm, ""}
		results = append(results, r)
		if bRes.exitCode != cRes.exitCode {
			t.Errorf("  claim: exit code: baseline=%d candidate=%d",
				bRes.exitCode, cRes.exitCode)
		} else if bNorm != cNorm {
			t.Errorf("  claim: output mismatch\n    baseline:  %q\n    candidate: %q",
				bNorm, cNorm)
		} else {
			t.Logf("  claim: PASS (exit=%d)", bRes.exitCode)
		}
	})

	// ── 5. bd close <id> ────────────────────────────────────────────────
	t.Run("close", func(t *testing.T) {
		bWS := newWorkspace(t, baselineBin)
		cWS := newWorkspace(t, candidateBin)
		bID := createInWS(t, baselineBin, bWS.dir, []string{
			"--title", "close-me", "--type", "task", "-p", "2",
		})
		cID := createInWS(t, candidateBin, cWS.dir, []string{
			"--title", "close-me", "--type", "task", "-p", "2",
		})
		bRes := runOracleCmd(baselineBin, bWS.dir, []string{"close", bID, "--reason", "done"})
		cRes := runOracleCmd(candidateBin, cWS.dir, []string{"close", cID, "--reason", "done"})
		bNorm := defaultNormalizer(bRes.stdout + bRes.stderr)
		cNorm := defaultNormalizer(cRes.stdout + cRes.stderr)
		passed := (bRes.exitCode == cRes.exitCode) && (bNorm == cNorm)
		r := oracleResult{"close", passed, bRes, cRes, bNorm, cNorm, ""}
		results = append(results, r)
		if bRes.exitCode != cRes.exitCode {
			t.Errorf("  close: exit code: baseline=%d candidate=%d",
				bRes.exitCode, cRes.exitCode)
		} else if bNorm != cNorm {
			t.Errorf("  close: output mismatch\n    baseline:  %q\n    candidate: %q",
				bNorm, cNorm)
		} else {
			t.Logf("  close: PASS (exit=%d)", bRes.exitCode)
		}
	})

	// ── 6. bd create "epic1" -t epic ────────────────────────────────────
	t.Run("create_epic", func(t *testing.T) {
		bWS := newWorkspace(t, baselineBin)
		cWS := newWorkspace(t, candidateBin)
		r := runAndCompare(t, "create_epic",
			[]string{"create", "--silent", "epic1", "-t", "epic"},
			bWS.dir, cWS.dir, idOutputNormalizer)
		results = append(results, r)
	})

	// ── 7. bd dep add <child> <parent> ──────────────────────────────────
	t.Run("dep_add", func(t *testing.T) {
		bWS := newWorkspace(t, baselineBin)
		cWS := newWorkspace(t, candidateBin)
		bChild := createInWS(t, baselineBin, bWS.dir, []string{
			"--title", "child", "--type", "task", "-p", "2",
		})
		bParent := createInWS(t, baselineBin, bWS.dir, []string{
			"--title", "parent", "--type", "task", "-p", "1",
		})
		cChild := createInWS(t, candidateBin, cWS.dir, []string{
			"--title", "child", "--type", "task", "-p", "2",
		})
		cParent := createInWS(t, candidateBin, cWS.dir, []string{
			"--title", "parent", "--type", "task", "-p", "1",
		})
		bRes := runOracleCmd(baselineBin, bWS.dir, []string{"dep", "add", bChild, bParent})
		cRes := runOracleCmd(candidateBin, cWS.dir, []string{"dep", "add", cChild, cParent})
		bNorm := defaultNormalizer(bRes.stdout + bRes.stderr)
		cNorm := defaultNormalizer(cRes.stdout + cRes.stderr)
		passed := (bRes.exitCode == cRes.exitCode) && (bNorm == cNorm)
		r := oracleResult{"dep_add", passed, bRes, cRes, bNorm, cNorm, ""}
		results = append(results, r)
		if bRes.exitCode != cRes.exitCode {
			t.Errorf("  dep_add: exit code: baseline=%d candidate=%d",
				bRes.exitCode, cRes.exitCode)
		} else if bNorm != cNorm {
			t.Errorf("  dep_add: output mismatch\n    baseline:  %q\n    candidate: %q",
				bNorm, cNorm)
		} else {
			t.Logf("  dep_add: PASS (exit=%d)", bRes.exitCode)
		}
	})

	// ── 8. bd ready ────────────────────────────────────────────────────
	t.Run("ready", func(t *testing.T) {
		bWS := newWorkspace(t, baselineBin)
		cWS := newWorkspace(t, candidateBin)
		// Create a P0 issue so there is ready work both sides
		createInWS(t, baselineBin, bWS.dir, []string{
			"--title", "urgent-task", "--type", "task", "-p", "0",
		})
		createInWS(t, candidateBin, cWS.dir, []string{
			"--title", "urgent-task", "--type", "task", "-p", "0",
		})
		r := runAndCompare(t, "ready",
			[]string{"ready", "--json", "-n", "0"},
			bWS.dir, cWS.dir, nil)
		results = append(results, r)
	})

	// ── 9. bd note <id> "comment" ──────────────────────────────────────
	t.Run("note", func(t *testing.T) {
		bWS := newWorkspace(t, baselineBin)
		cWS := newWorkspace(t, candidateBin)
		bID := createInWS(t, baselineBin, bWS.dir, []string{
			"--title", "note-me", "--type", "task", "-p", "2",
		})
		cID := createInWS(t, candidateBin, cWS.dir, []string{
			"--title", "note-me", "--type", "task", "-p", "2",
		})
		bRes := runOracleCmd(baselineBin, bWS.dir, []string{"update", bID, "--notes", "oracle comment"})
		cRes := runOracleCmd(candidateBin, cWS.dir, []string{"update", cID, "--notes", "oracle comment"})
		bNorm := defaultNormalizer(bRes.stdout + bRes.stderr)
		cNorm := defaultNormalizer(cRes.stdout + cRes.stderr)
		passed := (bRes.exitCode == cRes.exitCode) && (bNorm == cNorm)
		r := oracleResult{"note", passed, bRes, cRes, bNorm, cNorm, ""}
		results = append(results, r)
		if bRes.exitCode != cRes.exitCode {
			t.Errorf("  note: exit code: baseline=%d candidate=%d",
				bRes.exitCode, cRes.exitCode)
		} else if bNorm != cNorm {
			t.Errorf("  note: output mismatch\n    baseline:  %q\n    candidate: %q",
				bNorm, cNorm)
		} else {
			t.Logf("  note: PASS (exit=%d)", bRes.exitCode)
		}
	})

	// ── 10. bd update <id> --notes "append" ────────────────────────────
	t.Run("update_notes_append", func(t *testing.T) {
		bWS := newWorkspace(t, baselineBin)
		cWS := newWorkspace(t, candidateBin)
		bID := createInWS(t, baselineBin, bWS.dir, []string{
			"--title", "append-notes", "--type", "task", "-p", "2",
			"--notes", "initial",
		})
		cID := createInWS(t, candidateBin, cWS.dir, []string{
			"--title", "append-notes", "--type", "task", "-p", "2",
			"--notes", "initial",
		})
		bRes := runOracleCmd(baselineBin, bWS.dir, []string{"update", bID, "--notes", "appended"})
		cRes := runOracleCmd(candidateBin, cWS.dir, []string{"update", cID, "--notes", "appended"})
		bNorm := defaultNormalizer(bRes.stdout + bRes.stderr)
		cNorm := defaultNormalizer(cRes.stdout + cRes.stderr)
		passed := (bRes.exitCode == cRes.exitCode) && (bNorm == cNorm)
		r := oracleResult{"update_notes_append", passed, bRes, cRes, bNorm, cNorm, ""}
		results = append(results, r)
		if bRes.exitCode != cRes.exitCode {
			t.Errorf("  update_notes_append: exit code: baseline=%d candidate=%d",
				bRes.exitCode, cRes.exitCode)
		} else if bNorm != cNorm {
			t.Errorf("  update_notes_append: output mismatch\n    baseline:  %q\n    candidate: %q",
				bNorm, cNorm)
		} else {
			t.Logf("  update_notes_append: PASS (exit=%d)", bRes.exitCode)
		}
	})

	// ── Summary ──────────────────────────────────────────────────────────
	t.Log("")
	t.Log("===== Oracle A Conformance Report =====")
	passed := 0
	for _, r := range results {
		if r.passed {
			passed++
			t.Logf("  PASS: %s", r.name)
		} else {
			t.Errorf("  FAIL: %s (%s)", r.name, r.failMsg)
		}
	}
	t.Logf("  Total: %d | Passed: %d | Failed: %d",
		len(results), passed, len(results)-passed)
}
