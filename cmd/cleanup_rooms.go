package cmd

import (
	"fmt"
	"time"

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
	WorkersCount         int    `mapstructure:"workers-count"`
	AbandonedDays        int    `mapstructure:"abandoned-days"`
	NoCacheCleanup       bool   `mapstructure:"no-cache-cleanup"`
	FilterOnlyForUserID  string `mapstructure:"filter-only-for-user-id"`
}

var cleanupRoomsCmd = &cobra.Command{
	Use:   "cleanup-rooms",
	Short: "Delete rooms without users",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := CreateConfigFromViper[CleanupRoomsCmdConfig](viper.GetViper(), cmd.Flags())
		if err != nil {
			return err
		}
		if cfg.WorkersCount < 1 {
			return fmt.Errorf("workers-count must be greater than zero")
		}
		if cfg.AbandonedDays < 1 {
			return fmt.Errorf("abandoned-days must be greater than zero")
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

		activityCache, activityCacheCloser, err := synapse.NewRoomActivityCache(cmd.Context(), cfg.PostgresDSN)
		if err != nil {
			return fmt.Errorf("can't create room activity cache: %w", err)
		}
		defer activityCacheCloser.Close()

		abandonedBefore := time.Now().Add(-time.Duration(cfg.AbandonedDays) * 24 * time.Hour)
		iterator := synapse.NewRoomCleanupIterator(synapseClient, activityCache)

		return processor.NewRoomCleaner(logger, synapseClient, iterator, cfg.WorkersCount).
			Process(cmd.Context(), processor.RoomCleanerOptions{
				DoRealJob:           doRealJob,
				AbandonedBefore:     abandonedBefore,
				NoCacheCleanup:      cfg.NoCacheCleanup,
				FilterOnlyForUserID: id.UserID(cfg.FilterOnlyForUserID),
			})
	},
}

func init() {
	cleanupRoomsCmd.Flags().String("synapse-homeserver-url", "", "Synapse Homeserver URL")
	cleanupRoomsCmd.Flags().String("synapse-user-id", "", "Synapse UserID")
	cleanupRoomsCmd.Flags().String("synapse-access-token", "", "Synapse Access Token")
	cleanupRoomsCmd.Flags().String("postgres-dsn", "", "PostgreSQL DSN for room activity cache")

	cleanupRoomsCmd.Flags().Int("abandoned-days", 458, "Rooms with no messages for this many days are cleanup candidates")
	cleanupRoomsCmd.Flags().String("filter-only-for-user-id", "", "When set, only check rooms joined by this user ID")

	cleanupRoomsCmd.Flags().Int("workers-count", 4, "Number of room cleanup workers")
	cleanupRoomsCmd.Flags().Bool("no-cache-cleanup", false, "Write candidates to cache and skip eviction (for analytics before real deletion)")
	cleanupRoomsCmd.Flags().Bool("do-real-job", false, "Without this flag to action will be performed")

	rootCmd.AddCommand(cleanupRoomsCmd)
}
