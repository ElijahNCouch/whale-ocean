package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/usewhale/whale/internal/app"
	"github.com/usewhale/whale/internal/attachments"
	"github.com/usewhale/whale/internal/defaults"
	"github.com/usewhale/whale/internal/session"
	whaleworktree "github.com/usewhale/whale/internal/worktree"
)

func newExecCmd(opts *cliOptions) *cobra.Command {
	var jsonOutput bool
	var timeoutSec int
	var attachPaths []string
	var sessionID string
	var mode string
	c := &cobra.Command{
		Use:   "exec [prompt]",
		Short: "Run a single prompt non-interactively",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runExecE(cmd, opts, args, jsonOutput, timeoutSec, attachPaths, sessionID, mode)
			if err == nil || !jsonOutput {
				return err
			}
			if _, ok := err.(ExitError); ok {
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				return err
			}
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			if err := writeExecErrorJSON(cmd.OutOrStdout(), err); err != nil {
				return err
			}
			return ExitError{Code: 1}
		},
	}
	c.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	c.Flags().IntVar(&timeoutSec, "timeout-sec", 0, "Optional timeout in seconds for this exec run")
	c.Flags().StringArrayVar(&attachPaths, "attach", nil, "Attach a local file to the prompt")
	c.Flags().StringVar(&sessionID, "session", "", "Resume an existing session by id (default: create a new session)")
	c.Flags().StringVar(&mode, "mode", "", "Run in a specific mode (agent|ask|plan); overrides the session's saved mode for this run and persists on resume (default: agent for new sessions)")
	c.Long = `Run a single prompt non-interactively and exit.

Session and mode:
  --session ID   Resume an existing session so later rounds share its history.
                 A worktree attached to the session is re-entered; an explicit
                 --worktree must match the session's record.
  --mode MODE    Run this round as agent, ask, or plan. When resuming, an
                 explicit mode overrides the session's saved mode and is saved
                 for later rounds; without it the saved mode is kept (missing
                 defaults to agent).
`
	return c
}

func newResumeCmd(opts *cliOptions) *cobra.Command {
	var last bool
	c := &cobra.Command{
		Use:   "resume [id]",
		Short: "Resume a session (open picker, use --last, or pass an id)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectWorktreeFlag(cmd); err != nil {
				return err
			}
			if err := prepareResumeWorktree(args, last, opts); err != nil {
				return err
			}
			if err := prepareCLIConfig(cmd, opts); err != nil {
				return err
			}
			start, err := resumeStartOptions(args, last)
			if err != nil {
				return err
			}
			return runLoop(opts, start)
		},
	}
	c.Flags().BoolVar(&last, "last", false, "Resume the most recent session without opening the picker")
	return c
}

func resumeStartOptions(args []string, last bool) (app.StartOptions, error) {
	if last && len(args) > 0 {
		return app.StartOptions{}, fmt.Errorf("usage: whale resume [--last] [id]")
	}
	if last {
		return app.StartOptions{}, nil
	}
	if len(args) == 1 {
		id := strings.TrimSpace(args[0])
		if id == "" {
			return app.StartOptions{}, fmt.Errorf("usage: whale resume [--last] [id]")
		}
		return app.StartOptions{SessionID: id}, nil
	}
	return app.StartOptions{ResumeMenu: true}, nil
}

func prepareResumeWorktree(args []string, last bool, opts *cliOptions) error {
	if len(args) == 0 && !last {
		return nil
	}
	workspaceRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get workspace: %w", err)
	}
	start, err := resumeStartOptions(args, last)
	if err != nil {
		return err
	}
	decision, err := app.ResolveResumeWorktree(opts.cfg, start, workspaceRoot)
	if err != nil {
		return err
	}
	sess := decision.Session
	if strings.TrimSpace(sess.Path) == "" {
		return nil
	}
	targetWorkspace := decision.TargetWorkspace
	if strings.TrimSpace(targetWorkspace) == "" {
		targetWorkspace = sess.Path
	}
	if err := os.Chdir(targetWorkspace); err != nil {
		return fmt.Errorf("enter resume worktree: %w", err)
	}
	opts.worktreeSession = sess
	return nil
}

func newDoctorCmd(opts *cliOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run Whale health checks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectWorktreeFlag(cmd); err != nil {
				return err
			}
			return runDoctor(cmd.OutOrStdout(), opts.cfg)
		},
	}
}

func newSetupCmd(opts *cliOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Choose a model provider and save its key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectWorktreeFlag(cmd); err != nil {
				return err
			}
			return runSetup(cmd.OutOrStdout(), cmd.InOrStdin(), opts.cfg.DataDir)
		},
	}
}

// runSetup walks the free providers first, because the common case is someone
// who wants a working agent without a billing account. A provider that needs
// no key finishes immediately; the rest offer to open their console so the
// key is a paste away rather than a search.
func runSetup(out io.Writer, in io.Reader, dataDir string) error {
	reader := bufio.NewReader(in)
	creds, err := app.LoadCredentials(dataDir)
	if err != nil {
		return err
	}

	all := defaults.Providers()
	fmt.Fprintln(out, "Whale setup — pick a provider")
	fmt.Fprintln(out)
	for i, p := range all {
		note := p.FreeTier
		if note == "" {
			note = "paid account"
		}
		status := ""
		if app.ProviderUsable(p.ID, creds) {
			status = "  [ready]"
		}
		fmt.Fprintf(out, "  %d) %-32s %s%s\n", i+1, p.Label, note, status)
	}
	if app.LocalModelRunning() {
		fmt.Fprintln(out, "\n  A local Ollama server is already running on this machine.")
	}
	fmt.Fprintf(out, "\nProvider [1-%d, enter for %d]: ", len(all), 1)

	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("read choice: %w", err)
	}
	choice := 1
	if v := strings.TrimSpace(line); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < 1 || n > len(all) {
			return fmt.Errorf("pick a number between 1 and %d", len(all))
		}
		choice = n
	}
	chosen := all[choice-1]

	if !chosen.NeedsKey() {
		if err := app.SaveCredentials(dataDir, creds); err != nil {
			return err
		}
		fmt.Fprintf(out, "\nUsing %s with model %s.\n", chosen.Label, chosen.DefaultModel())
		if chosen.ID == defaults.ProviderOllama && !app.LocalModelRunning() {
			fmt.Fprintf(out, "No server is listening yet. Install it from %s, then run:\n", chosen.ConsoleURL)
			fmt.Fprintf(out, "  ollama pull %s\n", chosen.DefaultModel())
		}
		fmt.Fprintf(out, "Set provider = \"%s\" in .whale/config.toml, or run: whale --provider %s\n", chosen.ID, chosen.ID)
		return nil
	}

	if chosen.ConsoleURL != "" {
		fmt.Fprintf(out, "\nGet a key at %s\n", chosen.ConsoleURL)
		fmt.Fprint(out, "Open that page now? [Y/n]: ")
		ans, ansErr := reader.ReadString('\n')
		a := strings.ToLower(strings.TrimSpace(ans))
		// Pressing enter means yes, but reaching the end of the input does
		// not: setup is scripted in tests and in provisioning, and neither
		// should have a browser window thrown at it.
		if ansErr == nil && (a == "" || a == "y" || a == "yes") {
			if openErr := app.OpenInBrowser(chosen.ConsoleURL); openErr != nil {
				fmt.Fprintf(out, "Could not open a browser (%v) — copy the link above.\n", openErr)
			}
		}
	}

	envKey := strings.TrimSpace(os.Getenv(chosen.KeyEnv))
	if envKey != "" {
		fmt.Fprintf(out, "%s is already set in this environment.\n", chosen.KeyEnv)
		fmt.Fprintf(out, "%s key (enter to reuse it): ", chosen.Label)
	} else {
		fmt.Fprintf(out, "%s key: ", chosen.Label)
	}
	line, err = reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("read api key: %w", err)
	}
	key := strings.TrimSpace(line)
	if key == "" {
		key = envKey
	}
	if err := app.ValidateProviderAPIKey(chosen.ID, key); err != nil {
		return err
	}
	if err := app.SaveCredentials(dataDir, creds.WithKey(chosen.ID, key)); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nSaved the %s key to %s\n", chosen.Label, filepath.Join(dataDir, "credentials.json"))
	fmt.Fprintf(out, "Model: %s · run `whale` to start.\n", chosen.DefaultModel())
	return nil
}

func runDoctor(out io.Writer, cfg app.Config) error {
	workspaceRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get workspace: %w", err)
	}
	report, err := app.RunDoctor(context.Background(), cfg, workspaceRoot)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "whale doctor")
	fmt.Fprintf(out, "  workspace: %s\n", report.Workspace)
	fmt.Fprintf(out, "  data dir: %s\n", report.DataDir)
	fmt.Fprintln(out)
	for _, check := range report.Checks {
		fmt.Fprintf(out, "  %s  %-12s %s\n", doctorBadge(check.Level), check.Label, check.Detail)
	}
	fmt.Fprintln(out)
	ok, warn, fail := report.Summary()
	fmt.Fprintf(out, "%d ok · %d warn · %d fail\n", ok, warn, fail)
	if fail > 0 {
		return ExitError{Code: 1}
	}
	return nil
}

func doctorBadge(level app.DoctorLevel) string {
	switch level {
	case app.DoctorOK:
		return "ok"
	case app.DoctorWarn:
		return "warn"
	default:
		return "fail"
	}
}

func runExecE(cmd *cobra.Command, opts *cliOptions, args []string, jsonOutput bool, timeoutSec int, attachPaths []string, sessionID, mode string) error {
	if flagChanged(cmd, "mode") {
		trimmed := strings.TrimSpace(mode)
		if trimmed == "" {
			return &app.InvalidModeError{Value: mode}
		}
		if _, err := session.ParseMode(mode); err != nil {
			return &app.InvalidModeError{Value: mode}
		}
	}
	currentWorkspace, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get workspace: %w", err)
	}
	// Read-only resume pre-validation before any side effect: the strict
	// session contract, the cross-workspace gate, and the explicit-worktree
	// match are all checked here, with the would-be worktree path resolved
	// without creating a branch, worktree, or ignore-file change.
	// prepareWorktree below creates an explicit worktree only after these
	// checks have passed.
	var target app.ResumeWorktreeDecision
	if sid := strings.TrimSpace(sessionID); sid != "" {
		start := app.StartOptions{SessionID: sid, ModeOverride: mode}
		if worktreeFlagChanged(cmd) {
			sess, err := whaleworktree.ResolveSession(currentWorkspace, opts.worktreeName)
			if err != nil {
				return err
			}
			start.Worktree = worktreeSessionFrom(sess)
		}
		target, err = app.ValidateResumeTarget(opts.cfg, start, currentWorkspace)
		if err != nil {
			return err
		}
	}
	if err := prepareWorktree(cmd, opts); err != nil {
		return err
	}
	// Enter the recorded workspace for a resumed worktree session. For an
	// explicit --worktree, prepareWorktree already changed into the worktree;
	// this additionally aligns the process directory with the recorded
	// workspace before the project configuration is loaded, so the round runs
	// with the config of the directory it actually executes in.
	if now, err := os.Getwd(); err != nil {
		return fmt.Errorf("get workspace: %w", err)
	} else if target.TargetWorkspace != "" && target.TargetWorkspace != now {
		if err := os.Chdir(target.TargetWorkspace); err != nil {
			return fmt.Errorf("enter resume worktree: %w", err)
		}
	}
	if err := prepareCLIConfig(cmd, opts); err != nil {
		return err
	}
	return runExec(cmd.OutOrStdout(), cmd.ErrOrStderr(), cmd.InOrStdin(), opts, args, jsonOutput, timeoutSec, attachPaths, sessionID, mode)
}

func writeExecErrorJSON(out io.Writer, err error) error {
	res := app.ExecResult{Status: "error", Error: err.Error()}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

func runExec(out io.Writer, errOut io.Writer, in io.Reader, opts *cliOptions, args []string, jsonOutput bool, timeoutSec int, attachPaths []string, sessionID, mode string) error {
	prompt, err := readExecPrompt(in, args)
	if err != nil {
		return err
	}
	start := app.StartOptions{NewSession: true, Worktree: opts.worktreeSession, ModeOverride: mode}
	if sid := strings.TrimSpace(sessionID); sid != "" {
		start = app.StartOptions{SessionID: sid, Worktree: opts.worktreeSession, ModeOverride: mode}
		currentWorkspace, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get workspace: %w", err)
		}
		target, err := app.ValidateResumeTarget(opts.cfg, start, currentWorkspace)
		if err != nil {
			return err
		}
		if target.TargetWorkspace != "" && target.TargetWorkspace != currentWorkspace {
			if err := os.Chdir(target.TargetWorkspace); err != nil {
				return fmt.Errorf("enter resume worktree: %w", err)
			}
		}
		if strings.TrimSpace(start.Worktree.Path) == "" && target.Session.Path != "" {
			start.Worktree = target.Session
		}
		if err := app.CommitStartState(opts.cfg, start, currentWorkspace, target); err != nil {
			return err
		}
	}

	ctx := context.Background()
	if timeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
		defer cancel()
	}

	res, execErr := app.RunExecWithAttachments(ctx, opts.cfg, start, prompt, attachmentSourcesFromPaths(attachPaths))
	if jsonOutput {
		if err := writeExecJSON(out, res); err != nil {
			return err
		}
		if execErr != nil {
			return ExitError{Code: 1}
		}
		return nil
	}
	if txt := res.TextOutput(); txt != "" {
		if _, err := io.WriteString(out, txt); err != nil {
			return err
		}
		if !strings.HasSuffix(txt, "\n") {
			if _, err := io.WriteString(out, "\n"); err != nil {
				return err
			}
		}
	}
	if execErr != nil {
		if strings.TrimSpace(res.Error) != "" {
			if _, err := fmt.Fprintln(errOut, res.Error); err != nil {
				return err
			}
		}
		return ExitError{Code: 1}
	}
	return nil
}

func attachmentSourcesFromPaths(paths []string) []attachments.Source {
	if len(paths) == 0 {
		return nil
	}
	out := make([]attachments.Source, 0, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		out = append(out, attachments.Source{Path: path})
	}
	return out
}

func readExecPrompt(in io.Reader, args []string) (string, error) {
	if len(args) == 1 {
		prompt := strings.TrimSpace(args[0])
		if prompt == "" {
			return "", fmt.Errorf("prompt is empty")
		}
		return prompt, nil
	}
	if f, ok := in.(*os.File); ok {
		if info, err := f.Stat(); err == nil && (info.Mode()&os.ModeCharDevice) != 0 {
			return "", fmt.Errorf("prompt is required")
		}
	}
	b, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("read prompt from stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(b))
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	return prompt, nil
}

func writeExecJSON(out io.Writer, res app.ExecResult) error {
	if err := res.Validate(); err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}
