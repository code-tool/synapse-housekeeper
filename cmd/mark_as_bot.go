package cmd

import (
	"fmt"

	"github.com/FZambia/viper-lite"
	"github.com/spf13/cobra"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/synapseadmin"

	"synapse-housekeeper/internal/synapse"
)

type MarkAsBotCmdConfig struct {
	SynapseHomeserverUrl string `mapstructure:"synapse-homeserver-url"`
	SynapseUserID        string `mapstructure:"synapse-user-id"`
	SynapseAccessToken   string `mapstructure:"synapse-access-token"`
}

var markAsBotCmd = &cobra.Command{
	Use:   "mark-as-bot",
	Short: "Set UserType to 'bot'",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := CreateConfigFromViper[MarkAsBotCmdConfig](viper.GetViper(), cmd.Flags())
		if err != nil {
			return err
		}

		userId, err := cmd.Flags().GetString("user-id")
		if err != nil {
			return err
		}

		synapseClient, err := synapse.
			NewClient(cfg.SynapseHomeserverUrl, id.UserID(cfg.SynapseUserID), cfg.SynapseAccessToken)

		if err != nil {
			return fmt.Errorf("can't create Synapse Client: %w", err)
		}

		return synapseClient.CreateOrModifyAccount(cmd.Context(), id.UserID(userId), synapseadmin.ReqCreateOrModifyAccount{
			UserType: "bot",
		})
	},
}

func init() {
	markAsBotCmd.Flags().String("synapse-homeserver-url", "", "Synapse Homeserver URL")
	markAsBotCmd.Flags().String("synapse-user-id", "", "Synapse UserID")
	markAsBotCmd.Flags().String("synapse-access-token", "", "Synapse Access Token")

	markAsBotCmd.Flags().String("user-id", "", "UserId in matrix format (@name:server.com) to mark as-bot")

	rootCmd.AddCommand(markAsBotCmd)
}
