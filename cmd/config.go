package cmd

import (
	"strings"

	"github.com/FZambia/viper-lite"
	"github.com/spf13/pflag"
)

func CreateConfigFromViper[T any](v *viper.Viper, flags *pflag.FlagSet) (*T, error) {
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	if err := v.BindPFlags(flags); err != nil {
		return nil, err
	}

	var cfg T
	err := v.Unmarshal(&cfg)

	return &cfg, err
}
