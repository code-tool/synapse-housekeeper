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
	PostgresDSN          string `mapstructure:"postgres-dsn"`
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

		if cfg.PostgresDSN != "" {
			activityCache, err := synapse.NewRoomActivityCachePostgres(cmd.Context(), cfg.PostgresDSN)
			if err != nil {
				return fmt.Errorf("can't create room activity cache: %w", err)
			}
			defer activityCache.Close()

			synapseClient.WithRoomActivityCache(activityCache)
		}

		return processor.NewRoomCleaner(logger, synapseClient).Process(cmd.Context(), doRealJob)
	},
}

func init() {
	cleanupRoomsCmd.Flags().String("synapse-homeserver-url", "", "Synapse Homeserver URL")
	cleanupRoomsCmd.Flags().String("synapse-user-id", "", "Synapse UserID")
	cleanupRoomsCmd.Flags().String("synapse-access-token", "", "Synapse Access Token")
	cleanupRoomsCmd.Flags().String("postgres-dsn", "", "PostgreSQL DSN for room activity cache")

	cleanupRoomsCmd.Flags().Bool("do-real-job", false, "Without this flag to action will be performed")

	rootCmd.AddCommand(cleanupRoomsCmd)
}
