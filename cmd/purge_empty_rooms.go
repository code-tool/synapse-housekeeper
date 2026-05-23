package cmd

import (
	"fmt"

	"github.com/FZambia/viper-lite"
	"github.com/spf13/cobra"
	"maunium.net/go/mautrix/id"

	"synapse-housekeeper/internal/processor"
	"synapse-housekeeper/internal/synapse"
)

type CleanupRoomsCmdConfig struct {
	SynapseHomeserverUrl string `mapstructure:"synapse-homeserver-url"`
	SynapseUserID        string `mapstructure:"synapse-user-id"`
	SynapseAccessToken   string `mapstructure:"synapse-access-token"`
}

var cleanupRoomsCmd = &cobra.Command{
	Use:   "cleanup-rooms",
	Short: "Delete rooms without users",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := CreateConfigFromViper[CleanupRoomsCmdConfig](viper.GetViper(), cmd.Flags())
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

		roomCleaner := processor.NewRoomCleaner(logger, synapseClient)

		return roomCleaner.Process(cmd.Context(), doRealJob)
	},
}

func init() {
	cleanupRoomsCmd.Flags().String("synapse-homeserver-url", "", "Synapse Homeserver URL")
	cleanupRoomsCmd.Flags().String("synapse-user-id", "", "Synapse UserID")
	cleanupRoomsCmd.Flags().String("synapse-access-token", "", "Synapse Access Token")

	cleanupRoomsCmd.Flags().Bool("do-real-job", !false, "Without this flag to action will be performed")

	rootCmd.AddCommand(cleanupRoomsCmd)
}
