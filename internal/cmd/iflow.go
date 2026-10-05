package cmd

import (
	"path/filepath"

	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/cpars-innovation/cpicli/pkg/ops"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewIFlowCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "iflow",
		Short: "Work with integration flow files (copy a template under a new ID)",
	}
	cp := &cobra.Command{
		Use:   "copy",
		Short: "Copy an integration flow under a new ID, name and sender address",
		Long: `Copy an integration flow (a template or any reference flow) into a local
directory and rename it, so that it can be uploaded as a new flow:

  META-INF/MANIFEST.MF  Bundle-SymbolicName (";singleton:=true" kept), Bundle-Name,
                        Bundle-Version (default 1.0.0); long values wrapped at 72 bytes
  metainfo.prop         description (the source's description is removed when
                        --description is not given)
  <SourceID>.iflw       renamed to <ID>.iflw (other model file names are kept)
  .project              project name
  sender addresses      HTTPS/SOAP path, ProcessDirect address, SFTP directory,
                        JMS queue, ...: in the model, or in parameters.prop when the
                        address is a {{parameter}}

Every sender address must get a new value (--address), or --keep-addresses must
be given: two deployed flows on the same HTTP path or ProcessDirect address fail
to deploy, and two flows polling the same directory or queue take each other's
messages. One --address NEW is enough when the flow has one sender address;
otherwise give --address OLD=NEW per address (OLD as in the source, e.g.
{{Orders_Path}}). A value containing "=" is always read as OLD=NEW.

Receivers, steps, scripts and parameters other than the addresses are copied
unchanged. The result lists files that still contain the source ID (process
names, scripts, log texts): check them. The source is read from the tenant
(--from, read only) or a local directory (--from-dir, offline). Nothing is
written to the tenant: upload the copy with 'cpictl update artifact'. On an
error the target directory is left as it was.`,
		Example: `  # template on the tenant, one HTTPS sender
  cpictl iflow copy --from Template_Sync_HTTPS --id SD_Orders_S4_Sync \
    --name "SD Orders to S/4 (sync)" --description "Order intake from the web shop" \
    --address /sd/orders/s4 --dir content/SD_Orders/SD_Orders_S4_Sync

  # local template with two senders (a parameterised HTTPS path and a ProcessDirect test entry)
  cpictl iflow copy --from-dir content/Templates/Template_Async --id FI_Invoices_In \
    --address '{{Inbound_Path}}=/fi/invoices' --address /test/Template_Async=/test/FI_Invoices_In

  # then
  cpictl update artifact --artifact-id SD_Orders_S4_Sync --package-id SD_Orders \
    --dir-artifact content/SD_Orders/SD_Orders_S4_Sync`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		Annotations:  map[string]string{annotationOffline: "from-dir"},
		RunE: func(cmd *cobra.Command, args []string) error {
			addresses, _ := cmd.Flags().GetStringArray("address")
			keep, _ := cmd.Flags().GetBool("keep-addresses")
			o := ops.IFlowCopyOptions{
				FromID: config.GetString(cmd, "from"), FromVersion: config.GetString(cmd, "from-version"), FromDir: config.GetString(cmd, "from-dir"),
				TargetDir: config.GetString(cmd, "dir"), ID: config.GetString(cmd, "id"), Name: config.GetString(cmd, "name"),
				Description: config.GetString(cmd, "description"), Version: config.GetString(cmd, "version"),
				Addresses: addresses, KeepAddresses: keep,
			}
			if o.TargetDir == "" {
				o.TargetDir = o.ID
				if o.FromDir != "" {
					o.TargetDir = filepath.Join(filepath.Dir(filepath.Clean(o.FromDir)), o.ID)
				}
			}
			var exe *httpclnt.HTTPExecuter
			if o.FromID != "" {
				exe = tenantExecuter(cmd)
			}
			res, err := ops.CopyIFlow(exe, o)
			if err != nil {
				return err
			}
			output.SetResult(cmd.Context(), res)
			log.Info().Msgf("Copied %s to %s as %s (%d files)", res.From, res.Dir, res.ID, res.Files)
			for _, c := range res.Changes {
				log.Info().Msg("  " + c)
			}
			for _, r := range res.Remaining {
				log.Warn().Msgf("still mentions %s: %s", res.SourceID, r)
			}
			return nil
		},
	}
	cp.Flags().String("from", "", "ID of the source flow on the tenant")
	cp.Flags().String("from-version", "active", "Version of the source flow on the tenant")
	cp.Flags().String("from-dir", "", "Local directory of the source flow (instead of --from)")
	cp.Flags().String("id", "", "ID of the new flow")
	cp.Flags().String("name", "", "Display name of the new flow (default: the ID)")
	cp.Flags().String("description", "", "Description of the new flow")
	cp.Flags().String("version", "1.0.0", "Version of the new flow")
	cp.Flags().String("dir", "", "Target directory, must not exist or be empty (default: <ID> next to --from-dir, or ./<ID>)")
	cp.Flags().StringArray("address", nil, "New sender address: NEW (one sender) or OLD=NEW (repeatable)")
	cp.Flags().Bool("keep-addresses", false, "Allow sender addresses that stay the same as in the source")
	_ = cp.MarkFlagRequired("id")
	c.AddCommand(cp)
	return c
}
