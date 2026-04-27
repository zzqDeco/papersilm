package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zzqDeco/papersilm/internal/config"
	"github.com/zzqDeco/papersilm/internal/version"
	"github.com/zzqDeco/papersilm/pkg/protocol"
)

var (
	rootLoadConfig             = loadConfig
	rootShouldUseTUI           = shouldUseTUI
	rootRunTUI                 = RunTUI
	rootBuildRuntime           = buildRuntime
	rootPrepareSessionSnapshot = prepareSessionSnapshot
	rootRunREPL                = RunREPL
)

func NewRootCommand(ctx context.Context) *cobra.Command {
	var (
		printTask      string
		sourceArgs     []string
		continueLatest bool
		resumeID       string
		outputFormat   string
		permissionMode string
		lang           string
		style          string
		configOnly     bool
	)

	cmd := &cobra.Command{
		Use:     "papersilm",
		Short:   "Paper-focused document agent CLI",
		Version: version.Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := rootLoadConfig()
			if err != nil {
				return err
			}
			if configOnly {
				path := config.ConfigPath(cfg.BaseDir)
				if err := config.Save(path, cfg); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "wrote default config to %s\n", path)
				return nil
			}
			mode := protocol.PermissionMode(permissionMode)
			if mode == "" {
				mode = cfg.PermissionMode
			}
			if lang == "" {
				lang = cfg.DefaultLang
			}
			if style == "" {
				style = cfg.DefaultStyle
			}

			if rootShouldUseTUI(protocol.OutputFormat(outputFormat), printTask) {
				if err := rootRunTUI(ctx, TUIOptions{
					Config:         cfg,
					ContinueLatest: continueLatest,
					ResumeID:       resumeID,
					Mode:           mode,
					Lang:           lang,
					Style:          style,
				}); err == nil {
					return nil
				} else if errors.Is(err, ErrTUIStartup) {
					fmt.Fprintf(cmd.ErrOrStderr(), "tui unavailable, falling back to plain repl: %v\n", err)
				} else {
					return err
				}
			}

			cfg, store, svc, out, err := rootBuildRuntime(ctx, outputFormat)
			if err != nil {
				return err
			}

			if printTask != "" {
				var snapshot protocol.SessionSnapshot
				switch {
				case resumeID != "":
					snapshot, err = svc.LoadSession(resumeID)
					if err != nil {
						return err
					}
				case continueLatest:
					snapshot, err = svc.LatestSession()
					if err != nil {
						return err
					}
				}
				req := protocol.ClientRequest{
					Task:           printTask,
					Sources:        sourceArgs,
					PermissionMode: mode,
					Language:       lang,
					Style:          style,
				}
				if snapshot.Meta.SessionID != "" {
					req.SessionID = snapshot.Meta.SessionID
				}
				result, err := svc.Execute(ctx, req)
				if err != nil {
					return err
				}
				return out.PrintResult(result)
			}

			snapshot, err := rootPrepareSessionSnapshot(ctx, svc, store, mode, lang, style, continueLatest, resumeID)
			if err != nil {
				return err
			}
			return rootRunREPL(ctx, svc, store, snapshot, out)
		},
	}

	cmd.Flags().StringVarP(&printTask, "print", "p", "", "single-shot task")
	cmd.Flags().StringArrayVar(&sourceArgs, "source", nil, "paper source (repeatable)")
	cmd.Flags().BoolVar(&continueLatest, "continue", false, "continue latest session")
	cmd.Flags().StringVar(&resumeID, "resume", "", "resume session by id")
	cmd.Flags().StringVar(&outputFormat, "output-format", string(protocol.OutputFormatText), "output format: text|json|stream-json")
	cmd.Flags().StringVar(&permissionMode, "permission-mode", string(protocol.PermissionModeConfirm), "permission mode: plan|confirm|auto")
	cmd.Flags().StringVar(&lang, "lang", "", "output language")
	cmd.Flags().StringVar(&style, "style", "", "output style")
	cmd.Flags().BoolVar(&configOnly, "config-init", false, "write default config and exit")
	cmd.AddCommand(newVersionCommand())
	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), version.Lines())
			return err
		},
	}
}
