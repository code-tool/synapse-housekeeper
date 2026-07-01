package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/FZambia/viper-lite"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	logger = zap.NewNop()
)

type RootCmdConfig struct {
	Debug    bool `mapstructure:"debug"`
	LogLevel int  `mapstructure:"log-level"`
}

var rootCmd = &cobra.Command{
	Use:   "synapse-housekeeper",
	Short: "Set of tools to clean up Synapse",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	logger = createLogger(false, int(zapcore.InfoLevel))

	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1)

	flagsParsed := false

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) (err error) {
		flagsParsed = true
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		cfg, err := CreateConfigFromViper[RootCmdConfig](viper.New(), rootCmd.PersistentFlags())
		if err != nil {
			return err
		}

		logger = createLoggerAndOverrideStdLog(cfg.Debug, cfg.LogLevel)

		return nil
	}

	go func() {
		errChan <- rootCmd.ExecuteContext(ctx)
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	shutdown := func(err error) {
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}

		if flagsParsed {
			logger.Error("command execution failed", zap.Error(err))
		}

		os.Exit(1)
	}

	select {
	case err := <-errChan:
		cancel()
		shutdown(err)
	case <-sigs:
		cancel()
		<-errChan
	}
}

func init() {
	rootCmd.PersistentFlags().Bool("debug", false, "Debug mode")
	rootCmd.PersistentFlags().Int("log-level", 1, "Log level")
}
