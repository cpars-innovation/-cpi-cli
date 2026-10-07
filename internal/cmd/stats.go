package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/internal/stats"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// statsResult is the JSON result of stats.
type statsResult struct {
	Path     string      `json:"path"`
	Enabled  bool        `json:"enabled"`
	Since    *time.Time  `json:"since,omitempty"`
	Entries  int         `json:"entries"`
	Commands []stats.Row `json:"commands"`
	Reset    bool        `json:"reset,omitempty"`
}

func NewStatsCommand() *cobra.Command {
	var since, source string
	var reset bool
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show local usage statistics (which commands and MCP tools run, how often, how long)",
		Long: `Show local usage statistics: runs, failures and durations per command and MCP tool.

Every command and MCP tool call appends one line (name, source cli/mcp, exit code,
duration, time) to $HOME/.cpictl/stats.jsonl. No arguments, hosts or artifact names
are recorded, and nothing is sent anywhere. The file is compacted to its newest half
when it reaches 1 MiB. CPICTL_STATS=off turns recording off; --reset deletes the file.`,
		Example: `  cpictl stats
  cpictl stats --since 7d --source mcp
  cpictl stats --reset`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		Annotations:  map[string]string{annotationOffline: "true", annotationNoStats: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			path := stats.Path()
			res := statsResult{Path: path, Enabled: path != "", Commands: []stats.Row{}}
			if path == "" {
				output.SetResult(cmd.Context(), res)
				log.Info().Msg("Usage statistics are off (CPICTL_STATS=off)")
				return nil
			}
			if reset {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return err
				}
				res.Reset = true
				output.SetResult(cmd.Context(), res)
				log.Info().Msgf("Deleted %s", path)
				return nil
			}
			var from time.Time
			if since != "" {
				d, err := parseSince(since)
				if err != nil {
					return output.Usagef("invalid --since %q (e.g. 24h, 7d)", since)
				}
				from = time.Now().Add(-d)
				res.Since = &from
			}
			entries, err := stats.Read(path, from)
			if err != nil {
				return err
			}
			if source != "" {
				kept := entries[:0]
				for _, e := range entries {
					if e.Source == source {
						kept = append(kept, e)
					}
				}
				entries = kept
			}
			res.Entries = len(entries)
			res.Commands = stats.Summarize(entries)
			output.SetResult(cmd.Context(), res)

			if len(entries) == 0 {
				log.Info().Msgf("No runs recorded yet (%s)", path)
				return nil
			}
			log.Info().Msgf("%-4s %-28s %6s %6s %9s %9s %9s  %s", "SRC", "COMMAND", "RUNS", "FAILED", "P50", "P95", "MAX", "LAST USED")
			for _, r := range res.Commands {
				log.Info().Msgf("%-4s %-28s %6d %6d %9s %9s %9s  %s", r.Source, r.Command, r.Runs, r.Failed,
					formatMs(r.P50Ms), formatMs(r.P95Ms), formatMs(r.MaxMs), r.LastUsed.Local().Format("2006-01-02 15:04"))
			}
			log.Info().Msgf("%d run(s) in %s", len(entries), path)
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Only runs in this period, e.g. 24h or 7d")
	cmd.Flags().StringVar(&source, "source", "", "Only cli or mcp")
	cmd.Flags().BoolVar(&reset, "reset", false, "Delete the statistics file")
	return cmd
}

// parseSince accepts Go durations plus a day suffix (7d).
func parseSince(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid days")
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err == nil && d < 0 {
		err = fmt.Errorf("negative")
	}
	return d, err
}

func formatMs(ms int64) string {
	return formatElapsed(time.Duration(ms) * time.Millisecond)
}
