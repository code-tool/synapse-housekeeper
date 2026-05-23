package cmd

import (
	"fmt"

	"github.com/FZambia/viper-lite"
	"github.com/spf13/cobra"
	"maunium.net/go/mautrix/id"

	"synapse-housekeeper/internal/processor"
	"synapse-housekeeper/internal/synapse"
)

type CleanupOldDevicesCmdConfig struct {
	SynapseHomeserverUrl string `mapstructure:"synapse-homeserver-url"`
	SynapseUserID        string `mapstructure:"synapse-user-id"`
	SynapseAccessToken   string `mapstructure:"synapse-access-token"`
}

var cleanupOldDevicesCmd = &cobra.Command{
	Use:   "cleanup-old-devices",
	Short: "Delete old devices from users",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := CreateConfigFromViper[CleanupOldDevicesCmdConfig](viper.GetViper(), cmd.Flags())
		if err != nil {
			return err
		}

		doRealJob, err := cmd.Flags().GetBool("do-real-job")
		if err != nil {
			return err
		}

		synapseClient, err := synapse.
			NewClient(cfg.SynapseHomeserverUrl, id.UserID(cfg.SynapseUserID), cfg.SynapseAccessToken)

		if err != nil {
			return fmt.Errorf("can't create Synapse Client: %w", err)
		}

		return processor.NewDeviceCleaner(logger, synapseClient).Process(cmd.Context(), doRealJob)
	},
}

func init() {
	cleanupOldDevicesCmd.Flags().String("synapse-homeserver-url", "", "Synapse Homeserver URL")
	cleanupOldDevicesCmd.Flags().String("synapse-user-id", "", "Synapse UserID")
	cleanupOldDevicesCmd.Flags().String("synapse-access-token", "", "Synapse Access Token")

	cleanupOldDevicesCmd.Flags().Bool("do-real-job", false, "Without this flag to action will be performed")

	rootCmd.AddCommand(cleanupOldDevicesCmd)
}
