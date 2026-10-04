package cmd

import (
	"strings"
	"time"

	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewLogsCommand() *cobra.Command {
	c := &cobra.Command{
		Use:          "logs",
		Short:        "Query message processing logs",
		SilenceUsage: true,
		Long: `Query message processing logs (MPL) of the runtime, newest first.

--since/--until take a duration back from now (30m, 2h, 1d) or an RFC 3339
timestamp. --wait polls until at least one matching message exists and all
matching messages reached a final status (COMPLETED, FAILED, ESCALATED,
CANCELLED, DISCARDED, ABANDONED); use it after sending a test message with
--since set to the send time. Exit code 6 if the wait times out.

Statuses: ` + strings.Join(cpi.MessageLogStatuses, ", "),
		Example: `  cpictl logs --artifact-id OrderIntake --since 1h
  cpictl logs --artifact-id OrderIntake --status FAILED --errors --output json
  cpictl logs --artifact-id OrderIntake --since 2m --wait 60s --errors
  cpictl logs get --message-guid AFq478Bblxi4wCjBcDb_G0vAGGZG`,
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := messageLogQuery(cmd)
			if err != nil {
				return err
			}
			exe := tenantExecuter(cmd)
			var res *ops.MessageLogList
			if wait, _ := cmd.Flags().GetDuration("wait"); wait > 0 {
				res, err = ops.WaitForMessageLogs(cmd.Context(), exe, q, wait, 5*time.Second)
			} else {
				res, err = ops.QueryMessageLogs(exe, q)
			}
			if res != nil {
				output.SetResult(cmd.Context(), res)
				for _, l := range res.Logs {
					end := ""
					if l.LogEnd != nil {
						end = l.LogEnd.Local().Format(time.DateTime)
					}
					log.Info().Str("messageGuid", l.MessageGuid).Msgf("%s  %-10s %s  %s%s", end, l.Status, l.ArtifactID, l.MessageGuid, errSuffix(firstLine(l.ErrorText)))
				}
				log.Info().Msgf("%d of %d message(s)", len(res.Logs), res.Total)
			}
			return err
		},
	}
	f := c.Flags()
	f.String("artifact-id", "", "Integration flow ID")
	f.StringSlice("status", nil, "Comma separated statuses, e.g. FAILED,RETRY")
	f.String("since", "", "Messages that ended after this time (duration like 1h or RFC 3339)")
	f.String("until", "", "Messages that started before this time (duration like 1h or RFC 3339)")
	f.String("correlation-id", "", "Correlation ID")
	f.String("application-message-id", "", "Application message ID")
	f.Int("top", 20, "Maximum number of messages (max 200)")
	f.Int("skip", 0, "Skip the first n messages")
	f.Bool("errors", false, "Include the error text of failed messages")
	f.Duration("wait", 0, "Wait up to this long for final messages (e.g. 60s)")

	get := &cobra.Command{
		Use:          "get",
		Short:        "Show one message: status, error text, custom headers, attachments",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			guid := config.GetString(cmd, "message-guid")
			d, err := ops.GetMessageLog(tenantExecuter(cmd), guid, 0)
			if err != nil {
				return err
			}
			output.SetResult(cmd.Context(), d)
			log.Info().Msgf("%s  %s  %s", d.MessageGuid, d.Status, d.ArtifactID)
			if d.ErrorText != "" {
				log.Info().Msgf("Error: %s", d.ErrorText)
			}
			for _, h := range d.CustomHeaderProperties {
				log.Info().Msgf("Header %s = %s", h.Name, h.Value)
			}
			for _, a := range d.Attachments {
				log.Info().Msgf("Attachment %s (%s, %d bytes)", a.Name, a.ContentType, a.Size)
			}
			return nil
		},
	}
	get.Flags().String("message-guid", "", "Message GUID")

	steps := &cobra.Command{
		Use:          "steps",
		Short:        "Show the processing steps of a message and the step that failed",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := ops.GetMessageSteps(tenantExecuter(cmd), config.GetString(cmd, "message-guid"))
			if err != nil {
				return err
			}
			output.SetResult(cmd.Context(), res)
			for _, r := range res.Runs {
				for _, st := range r.Steps {
					log.Info().Msgf("%-10s %-30s %s%s", st.Status, st.ModelStepID, st.Activity, errSuffix(st.Error))
				}
			}
			if res.FailedStep != nil {
				log.Warn().Msgf("Failed step: %s (%s)%s", res.FailedStep.ModelStepID, res.FailedStep.Activity, errSuffix(res.FailedStep.Error))
			}
			return nil
		},
	}
	steps.Flags().String("message-guid", "", "Message GUID")

	attachment := &cobra.Command{
		Use:          "attachment",
		Short:        "Download a log attachment (ID from 'logs get')",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := ops.GetMessageAttachment(tenantExecuter(cmd), config.GetString(cmd, "id"), contentLimit(cmd))
			if err != nil {
				return err
			}
			return emitContent(cmd, res, res.Content)
		},
	}
	attachment.Flags().String("id", "", "Attachment ID")
	addContentFlags(attachment)

	payload := &cobra.Command{
		Use:          "payload",
		Short:        "Download a persisted message (message store entry ID from 'logs get')",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := ops.GetMessageStoreEntry(tenantExecuter(cmd), config.GetString(cmd, "id"), contentLimit(cmd))
			if err != nil {
				return err
			}
			return emitContent(cmd, res, res.Content)
		},
	}
	payload.Flags().String("id", "", "Message store entry ID")
	addContentFlags(payload)

	c.AddCommand(get, steps, attachment, payload)
	return c
}

func messageLogQuery(cmd *cobra.Command) (ops.MessageLogQuery, error) {
	q := ops.MessageLogQuery{
		ArtifactID:           config.GetString(cmd, "artifact-id"),
		Statuses:             nonEmpty(config.GetStringSlice(cmd, "status")),
		CorrelationID:        config.GetString(cmd, "correlation-id"),
		ApplicationMessageID: config.GetString(cmd, "application-message-id"),
		Top:                  config.GetInt(cmd, "top"),
		Skip:                 config.GetInt(cmd, "skip"),
		IncludeErrors:        config.GetBool(cmd, "errors"),
	}
	var err error
	if q.Since, err = ops.ParseTimeArg(config.GetString(cmd, "since"), time.Now()); err != nil {
		return q, output.Usagef("invalid --since: %v", err)
	}
	if q.Until, err = ops.ParseTimeArg(config.GetString(cmd, "until"), time.Now()); err != nil {
		return q, output.Usagef("invalid --until: %v", err)
	}
	return q, nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
