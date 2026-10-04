package cmd

import (
	"github.com/cpars-innovation/cpicli/internal/config"
	"github.com/cpars-innovation/cpicli/pkg/cpi"
	"github.com/spf13/cobra"
)

// serviceDetails reads the tenant connection settings from the (config/env
// bound) persistent flags.
func serviceDetails(cmd *cobra.Command) *cpi.ServiceDetails {
	oauthHost := config.GetString(cmd, "oauth-host")
	if oauthHost == "" {
		return &cpi.ServiceDetails{
			Host:     config.GetString(cmd, "tmn-host"),
			Userid:   config.GetString(cmd, "tmn-userid"),
			Password: config.GetString(cmd, "tmn-password"),
		}
	}
	return &cpi.ServiceDetails{
		Host:              config.GetString(cmd, "tmn-host"),
		OauthHost:         oauthHost,
		OauthClientId:     config.GetString(cmd, "oauth-clientid"),
		OauthClientSecret: config.GetString(cmd, "oauth-clientsecret"),
		OauthPath:         config.GetString(cmd, "oauth-path"),
	}
}
