package cmd

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// addRuntimeAuthFlags adds the credentials used for runtime endpoints
// (sending test messages). They usually belong to a separate service key
// (Process Integration Runtime, plan integration-flow) with the role
// ESBMessaging.send.
func addRuntimeAuthFlags(c *cobra.Command) {
	c.Flags().String("runtime-oauth-host", "", "OAuth token server host for runtime endpoints (default: --oauth-host)")
	c.Flags().String("runtime-oauth-clientid", "", "OAuth client ID for runtime endpoints (default: the API credentials)")
	c.Flags().String("runtime-oauth-clientsecret", "", "OAuth client secret for runtime endpoints")
	c.Flags().String("runtime-userid", "", "User ID for Basic Auth on runtime endpoints")
	c.Flags().String("runtime-password", "", "Password for Basic Auth on runtime endpoints")
}

// runtimeDetails returns the runtime endpoint credentials, falling back to
// the API credentials when none are set.
func runtimeDetails(cmd *cobra.Command) *cpi.ServiceDetails {
	switch {
	case config.GetString(cmd, "runtime-oauth-clientid") != "":
		host := config.GetString(cmd, "runtime-oauth-host")
		if host == "" {
			host = config.GetString(cmd, "oauth-host")
		}
		return &cpi.ServiceDetails{OauthHost: host, OauthPath: config.GetString(cmd, "oauth-path"),
			OauthClientId: config.GetString(cmd, "runtime-oauth-clientid"), OauthClientSecret: config.GetString(cmd, "runtime-oauth-clientsecret")}
	case config.GetString(cmd, "runtime-userid") != "":
		return &cpi.ServiceDetails{Userid: config.GetString(cmd, "runtime-userid"), Password: config.GetString(cmd, "runtime-password")}
	}
	return serviceDetails(cmd)
}

func endpointExecuter(cmd *cobra.Command) ops.EndpointExecuterFunc {
	details := runtimeDetails(cmd)
	return func(url string) (*httpclnt.HTTPExecuter, string, error) {
		return cpi.NewEndpointExecuter(details, url)
	}
}

func NewSendCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "send",
		Short: "Send a test message to a deployed integration flow",
		Long: `Send a test message to an endpoint of a deployed integration flow and print
the HTTP status, the response and the message GUID. Only endpoint URLs that the
tenant lists for the flow (see 'endpoints') are used.

With --wait the command also waits for the message processing log and exits
with code 5 when the message did not complete. The message is processed like any
other, including calls to receivers: use test data on a development tenant.

Runtime endpoints usually need other credentials than the API: set
--runtime-oauth-clientid/--runtime-oauth-clientsecret (CPICTL_RUNTIME_OAUTH_*)
from a service key of plan integration-flow with role ESBMessaging.send.
Without them the API credentials are used.`,
		Example: `  # Send a file and wait for the outcome
  cpictl send --artifact-id OrderIntake --body-file order.xml --content-type application/xml --wait 60s

  # Body from stdin, extra header
  echo '{"id":1}' | cpictl send --artifact-id OrderIntake --body-file - --header X-Test=1`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := readBody(cmd)
			if err != nil {
				return err
			}
			headers := map[string]string{}
			for _, h := range config.GetStringSlice(cmd, "header") {
				k, v, ok := strings.Cut(h, "=")
				if !ok || k == "" {
					return output.Usagef("invalid --header %q, expected name=value", h)
				}
				headers[k] = v
			}
			wait, _ := cmd.Flags().GetDuration("wait")
			sent, err := ops.SendTestMessage(cmd.Context(), tenantExecuter(cmd), endpointExecuter(cmd), ops.TestMessage{
				ArtifactID: config.GetString(cmd, "artifact-id"), URL: config.GetString(cmd, "url"),
				Method: config.GetString(cmd, "method"), Body: body, ContentType: config.GetString(cmd, "content-type"),
				Headers: headers, Wait: wait, PollInterval: 5 * time.Second,
			})
			if sent != nil {
				output.SetResult(cmd.Context(), sent)
				log.Info().Msgf("%s answered HTTP %d, message %s", sent.URL, sent.HTTPStatus, sent.MessageGuid)
				if sent.Log != nil {
					log.Info().Msgf("Message status %s %s", sent.Log.Status, sent.Log.ErrorText)
				}
			}
			return err
		},
	}
	c.Flags().String("artifact-id", "", "Integration flow ID (deployed)")
	c.Flags().String("url", "", "Endpoint URL, only needed when the flow has several endpoints")
	c.Flags().String("method", "POST", "HTTP method")
	c.Flags().String("body", "", "Message body")
	c.Flags().String("body-file", "", "Read the message body from this file ('-' for stdin)")
	c.Flags().String("content-type", "", "Content-Type of the body")
	c.Flags().StringSlice("header", nil, "Additional header name=value (repeatable)")
	c.Flags().Duration("wait", 0, "Wait up to this long for the message processing log (e.g. 60s)")
	addRuntimeAuthFlags(c)
	_ = c.MarkFlagRequired("artifact-id")
	return c
}

func readBody(cmd *cobra.Command) ([]byte, error) {
	text, file := config.GetString(cmd, "body"), config.GetString(cmd, "body-file")
	switch {
	case text != "" && file != "":
		return nil, output.Usagef("use either --body or --body-file")
	case file == "-":
		return io.ReadAll(cmd.InOrStdin())
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, output.Usage(err)
		}
		return b, nil
	}
	return []byte(text), nil
}
