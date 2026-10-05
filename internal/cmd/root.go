package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/exitcode"
	"github.com/cpars-innovation/cpicli/internal/logger"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func NewCmdRoot(version string) *cobra.Command {
	// rootCmd represents the base command when called without any subcommands
	rootCmd := &cobra.Command{
		Use:     "cpictl",
		Version: version,
		Short:   "Build, deploy and operate SAP Cloud Integration content",
		Long: `cpictl builds, deploys and operates SAP Cloud Integration (CPI) content from
the command line, CI/CD pipelines and AI agents (cpictl mcp).

Tenant connection: --tmn-host plus OAuth (--oauth-host, --oauth-clientid,
--oauth-clientsecret) or Basic Auth (--tmn-userid, --tmn-password). Every flag
can also be set as CPICTL_<FLAG> environment variable or in $HOME/cpictl.yaml.

Use --output json for one machine-readable result document on stdout.
Exit codes: 0 ok, 2 usage, 3 auth, 4 tenant HTTP error, 5 failed, 6 timeout,
7 partial failure.`,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// You can bind cobra and viper in a few locations, but PersistencePreRunE on the root command works well
			return initializeConfig(cmd)
		},
	}

	rootCmd.PersistentFlags().String("config", "", "config file (default is $HOME/cpictl.yaml)")

	// Define cobra flags, the default value has the lowest (least significant) precedence
	rootCmd.PersistentFlags().String("tmn-host", "", "Tenant host of Cloud Integration (or API portal host for API Management)")
	rootCmd.PersistentFlags().String("tmn-userid", "", "User ID for Basic Auth")
	rootCmd.PersistentFlags().String("tmn-password", "", "Password for Basic Auth")
	rootCmd.PersistentFlags().String("oauth-host", "", "OAuth token server host")
	rootCmd.PersistentFlags().String("oauth-clientid", "", "Client ID for using OAuth")
	rootCmd.PersistentFlags().String("oauth-clientsecret", "", "Client Secret for using OAuth")
	rootCmd.PersistentFlags().String("oauth-path", "/oauth/token", "Path for OAuth token server")

	rootCmd.PersistentFlags().Bool("debug", false, "Show debug logs")
	rootCmd.PersistentFlags().String("output", output.FormatText, "Output format: text or json. With json the result is written to stdout as one JSON document and logs are written to stderr as JSON lines")

	rootCmd.MarkFlagsRequiredTogether("tmn-userid", "tmn-password")
	rootCmd.MarkFlagsRequiredTogether("oauth-host", "oauth-clientid", "oauth-clientsecret")

	return rootCmd
}

// NewCLI returns the root command with all subcommands attached.
func NewCLI(version string) *cobra.Command {
	rootCmd := NewCmdRoot(version)
	rootCmd.AddCommand(NewDeployCommand())
	rootCmd.AddCommand(NewUndeployCommand())
	rootCmd.AddCommand(NewStatusCommand())
	rootCmd.AddCommand(NewLogsCommand())
	rootCmd.AddCommand(NewValidateCommand())
	rootCmd.AddCommand(NewGuidelinesCommand())
	rootCmd.AddCommand(NewEndpointsCommand())
	rootCmd.AddCommand(NewResourcesCommand())
	rootCmd.AddCommand(NewDownloadCommand())
	rootCmd.AddCommand(NewCredentialsCommand())
	rootCmd.AddCommand(NewKeystoreCommand())
	rootCmd.AddCommand(NewPackagesCommand())
	rootCmd.AddCommand(NewArtifactsCommand())
	rootCmd.AddCommand(NewParamsCommand())
	rootCmd.AddCommand(NewSendCommand())
	rootCmd.AddCommand(NewDiscoverCommand())
	rootCmd.AddCommand(NewMCPCommand(version))
	syncCmd := NewSyncCommand()
	syncCmd.AddCommand(NewAPIProxyCommand())
	syncCmd.AddCommand(NewAPIProductCommand())
	rootCmd.AddCommand(syncCmd)
	updateCmd := NewUpdateCommand()
	updateCmd.AddCommand(NewArtifactCommand())
	updateCmd.AddCommand(NewPackageCommand())
	rootCmd.AddCommand(updateCmd)
	snapshotCmd := NewSnapshotCommand()
	snapshotCmd.AddCommand(NewRestoreCommand())
	rootCmd.AddCommand(snapshotCmd)
	rootCmd.AddCommand(NewPDSnapshotCommand())
	rootCmd.AddCommand(NewPDDeployCommand())
	rootCmd.AddCommand(NewConfigGenerateCommand())
	rootCmd.AddCommand(NewOrchestratorCommand())
	rootCmd.AddCommand(NewConfigureCommand())
	return rootCmd
}

// Execute runs the CLI with the process arguments and exits with the
// contract exit code. This is called by main.main().
func Execute(version, buildTime string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := Run(ctx, os.Args[1:], os.Stdout, os.Stderr, version, buildTime)
	stop()
	os.Exit(code)
}

// Run executes the CLI and returns the exit code (see internal/exitcode).
// Results go to stdout, logs to stderr.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, version, buildTime string) int {
	rootCmd := NewCLI(version)
	rootCmd.SetVersionTemplate(fmt.Sprintf("cpictl version {{.Version}} (built %s)\n", buildTime))
	rootCmd.SetArgs(args)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return output.Usage(err) })

	// The format is needed before cobra has parsed the flags (e.g. to report
	// a flag parsing error as JSON)
	format := scanOutputFormat(args)
	logger.Init(stderr, format == output.FormatJSON, false)

	// Errors raised before a command's RunE starts are usage/config errors
	started := false
	walkCommands(rootCmd, func(c *cobra.Command) {
		if format == output.FormatJSON {
			c.SilenceUsage = true // usage text would break the JSON-lines stderr contract
		}
		if run := c.RunE; run != nil {
			c.RunE = func(cmd *cobra.Command, args []string) error {
				started = true
				return run(cmd, args)
			}
		}
	})

	ctx = output.WithResultHolder(ctx)
	cmd, err := rootCmd.ExecuteContextC(ctx)
	if cmd != nil {
		if f := cmd.Flags().Lookup("output"); f != nil && f.Changed {
			format = outputFormat(cmd)
		}
	}

	code := output.ExitCode(err)
	if err != nil && !started && code == exitcode.Error {
		code = exitcode.Usage
	}
	if err != nil {
		log.Error().Int("exitCode", code).Msg(logger.GetErrorDetails(err))
	}

	// --help/--version and commands without RunE produce no result document
	ownsStdout := cmd != nil && cmd.Annotations[annotationNoEnvelope] == "true" && started
	if format == output.FormatJSON && (started || err != nil) && !ownsStdout {
		env := output.Envelope{Command: commandName(cmd), OK: code == exitcode.OK, ExitCode: code, Result: output.Result(ctx)}
		if err != nil {
			env.Error = err.Error()
		}
		if werr := output.WriteEnvelope(stdout, env); werr != nil && code == exitcode.OK {
			code = exitcode.Error
		}
	}
	return code
}

func walkCommands(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, sub := range c.Commands() {
		walkCommands(sub, fn)
	}
}

func commandName(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()))
}

// scanOutputFormat finds --output in raw arguments (or CPICTL_OUTPUT).
func scanOutputFormat(args []string) string {
	value := os.Getenv(envPrefix + "_OUTPUT")
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--output" && i+1 < len(args) {
			value = args[i+1]
		} else if v, ok := strings.CutPrefix(arg, "--output="); ok {
			value = v
		}
	}
	if value == output.FormatJSON {
		return output.FormatJSON
	}
	return output.FormatText
}

// outputFormat returns the effective output format of a parsed command.
func outputFormat(cmd *cobra.Command) string {
	if config.GetString(cmd, "output") == output.FormatJSON {
		return output.FormatJSON
	}
	return output.FormatText
}

// envPrefix is the prefix of the environment variables bound to flags.
const envPrefix = "CPICTL"

// legacyEnvPrefix is the FlashPipe prefix, only used to warn about old settings.
const legacyEnvPrefix = "FLASHPIPE"

// annotationOffline marks commands that do not talk to a tenant: "true", or the
// name of a flag that makes the command offline when set (e.g. discover --dir).
const annotationOffline = "cpicli/offline"

// legacySettingsHint warns about FlashPipe-era settings that are no longer read.
func legacySettingsHint(cfgFile string) string {
	var found []string
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, legacyEnvPrefix+"_") {
			found = append(found, name)
		}
	}
	var hints []string
	if len(found) > 0 {
		sort.Strings(found)
		hints = append(hints, fmt.Sprintf("FLASHPIPE_* environment variables are not read, rename them to CPICTL_* (found: %s)", strings.Join(found, ", ")))
	}
	if cfgFile == "" {
		if home, err := os.UserHomeDir(); err == nil {
			_, errOld := os.Stat(filepath.Join(home, "flashpipe.yaml"))
			_, errNew := os.Stat(filepath.Join(home, "cpictl.yaml"))
			if errOld == nil && errNew != nil {
				hints = append(hints, "$HOME/flashpipe.yaml is not read, rename it to $HOME/cpictl.yaml")
			}
		}
	}
	if len(hints) == 0 {
		return ""
	}
	return strings.Join(hints, "; ") + " (see docs/migrating-from-flashpipe.md)"
}

func hintSuffix(hint string) string {
	if hint == "" {
		return ""
	}
	return ". Note: " + hint
}

// readConfigFile reads the --config file or, without one, $HOME/cpictl.yaml if it
// exists. The default file is set by its full name: a search by base name would also
// pick up an extension-less $HOME/cpictl (for example the binary itself).
func readConfigFile(cfgFile string) error {
	if cfgFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		cfgFile = filepath.Join(home, "cpictl.yaml")
		if _, err := os.Stat(cfgFile); err != nil {
			return nil
		}
	}
	viper.SetConfigFile(cfgFile)
	return viper.ReadInConfig()
}

func initializeConfig(cmd *cobra.Command) error {
	cfgFile := config.GetString(cmd, "config")
	if err := readConfigFile(cfgFile); err != nil {
		return output.Usage(err)
	}

	viper.SetEnvPrefix(envPrefix)

	// Environment variables can't have dashes in them, so bind them to their equivalent
	// keys with underscores, e.g. --artifact-id to CPICTL_ARTIFACT_ID
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// Bind to environment variables
	viper.AutomaticEnv()

	// Bind the current command's flags to viper
	bindFlags(cmd)

	// Set debug flag from command line to viper
	if !viper.IsSet("debug") {
		viper.Set("debug", config.GetBool(cmd, "debug"))
	}

	format := config.GetString(cmd, "output")
	if format != output.FormatText && format != output.FormatJSON {
		return output.Usagef("invalid value for --output = %v (allowed: text, json)", format)
	}

	logger.Init(cmd.ErrOrStderr(), format == output.FormatJSON, viper.GetBool("debug"))
	legacy := legacySettingsHint(cfgFile)
	if legacy != "" {
		log.Warn().Msg(legacy)
	}

	if offline := cmd.Annotations[annotationOffline]; offline != "true" && (offline == "" || config.GetString(cmd, offline) == "") {
		hasAuth := config.GetString(cmd, "oauth-host") != "" || config.GetString(cmd, "tmn-userid") != ""
		switch {
		case config.GetString(cmd, "tmn-host") == "" && !hasAuth:
			return output.Usagef("no tenant configured: set --tmn-host and OAuth (--oauth-host, --oauth-clientid, --oauth-clientsecret) or Basic Auth (--tmn-userid, --tmn-password)%s", hintSuffix(legacy))
		case config.GetString(cmd, "tmn-host") == "":
			return output.Usagef("required flag \"tmn-host\" not set%s", hintSuffix(legacy))
		case !hasAuth:
			return output.Usagef("required flag \"tmn-userid\" (Basic Auth) or \"oauth-host\" (OAuth) not set%s", hintSuffix(legacy))
		}
	}

	return nil
}

// Bind each cobra flag to its associated viper configuration (config file and environment variable)
func bindFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		configName := f.Name
		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && viper.IsSet(configName) {
			val := viper.Get(configName)
			cmd.Flags().Set(configName, fmt.Sprintf("%v", val))
		}
	})
}
