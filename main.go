package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aqyuki/felm/internal/app"
	"github.com/aqyuki/felm/internal/app/handler"
	"github.com/aqyuki/felm/pkg/discord"
	"github.com/aqyuki/felm/pkg/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var rootCmd = &cobra.Command{
	Use:   "felm",
	Short: "felm is a discord bot",
	RunE: func(_ *cobra.Command, _ []string) error {
		logger := logging.NewLoggerFromEnv()
		defer logger.Sync()

		ctx := logging.WithLogger(context.Background(), logger)
		ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer cancel()

		logger.Info("felm is starting setup")

		logger.Info("try to load application profile")
		profile := &app.Profile{
			Token:   viper.GetString("token"),
			Timeout: viper.GetDuration("timeout"),
		}
		logger.Info("application profile was loaded")

		app := discord.NewConn(profile.Token,
			discord.WithBaseContext(ctx),
			discord.WithHandlerTimeout(profile.Timeout),
			discord.WithMessageCreateHandler(handler.ExpandMessageLink),
		)

		logger.Info("starting application")
		if err := app.Open(); err != nil {
			logger.Error("failed to open connection", zap.Error(err))
			return err
		}

		<-ctx.Done()
		logger.Info("signal received, closing application")

		if err := app.Close(); err != nil {
			logger.Error("failed to close connection", zap.Error(err))
			return err
		}
		logger.Info("application stopped successfully")
		return nil
	},
}

func init() {
	viper.SetDefault("timeout", 5*time.Second)

	rootCmd.PersistentFlags().String("token", "", "token is a Discord bot token. It or FELM_DISCORD_TOKEN is required.")
	rootCmd.PersistentFlags().Duration("timeout", 5*time.Second, "timeout is a duration for HTTP client timeout. It or FELM_TIMEOUT is optional.")

	if err := viper.BindPFlag("token", rootCmd.PersistentFlags().Lookup("token")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("timeout", rootCmd.PersistentFlags().Lookup("timeout")); err != nil {
		panic(err)
	}

	viper.SetEnvPrefix("felm")
	viper.AutomaticEnv()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
