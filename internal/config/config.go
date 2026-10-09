package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func GetString(cmd *cobra.Command, flagName string) string {
	val, _ := cmd.Flags().GetString(flagName)
	return val
}

func GetStringSlice(cmd *cobra.Command, flagName string) []string {
	val, _ := cmd.Flags().GetStringSlice(flagName)
	return val
}

func GetInt(cmd *cobra.Command, flagName string) int {
	val, _ := cmd.Flags().GetInt(flagName)
	return val
}

func GetBool(cmd *cobra.Command, flagName string) bool {
	val, _ := cmd.Flags().GetBool(flagName)
	return val
}

// minSecretLength is the shortest secret value looked for in inputs: a
// shorter one (a mock's "mock") matches ordinary paths by chance.
const minSecretLength = 8

// verifyNoSensitiveContent rejects an input (a path from a flag, config file
// or expanded environment variable) that contains the tenant password or the
// OAuth client secret, so that a secret never ends up in a file path or log.
// User IDs and client IDs are not secrets and are not checked: a short one
// such as "cpi" would block ordinary paths like ".cpi/".
func verifyNoSensitiveContent(input string) (bool, error) {
	for _, param := range []string{"tmn-password", "oauth-clientsecret"} {
		secret := viper.GetString(param)
		if len(secret) >= minSecretLength && strings.Contains(input, secret) {
			return false, fmt.Errorf("Input contains sensitive content from configuration parameter %v", param)
		}
	}
	return true, nil
}

// GetStringWithFallback reads a string value from command flag,
// falling back to a nested config key if the flag wasn't explicitly set
func GetStringWithFallback(cmd *cobra.Command, flagName, configKey string) string {
	// Check if flag was explicitly set on command line
	if cmd.Flags().Changed(flagName) {
		return GetString(cmd, flagName)
	}

	// Try to get from nested config key
	if viper.IsSet(configKey) {
		return viper.GetString(configKey)
	}

	// Fall back to flag default
	return GetString(cmd, flagName)
}

// GetBoolWithFallback reads a bool value from command flag,
// falling back to a nested config key if the flag wasn't explicitly set
func GetBoolWithFallback(cmd *cobra.Command, flagName, configKey string) bool {
	// Check if flag was explicitly set on command line
	if cmd.Flags().Changed(flagName) {
		return GetBool(cmd, flagName)
	}

	// Try to get from nested config key
	if viper.IsSet(configKey) {
		return viper.GetBool(configKey)
	}

	// Fall back to flag default
	return GetBool(cmd, flagName)
}

// GetIntWithFallback reads an int value from command flag,
// falling back to a nested config key if the flag wasn't explicitly set
func GetIntWithFallback(cmd *cobra.Command, flagName, configKey string) int {
	// Check if flag was explicitly set on command line
	if cmd.Flags().Changed(flagName) {
		return GetInt(cmd, flagName)
	}

	// Try to get from nested config key
	if viper.IsSet(configKey) {
		return viper.GetInt(configKey)
	}

	// Fall back to flag default
	return GetInt(cmd, flagName)
}

// GetStringSliceWithFallback reads a string slice value from command flag,
// falling back to a nested config key if the flag wasn't explicitly set
func GetStringSliceWithFallback(cmd *cobra.Command, flagName, configKey string) []string {
	// Check if flag was explicitly set on command line
	if cmd.Flags().Changed(flagName) {
		return GetStringSlice(cmd, flagName)
	}

	// Try to get from nested config key
	if viper.IsSet(configKey) {
		return viper.GetStringSlice(configKey)
	}

	// Fall back to flag default
	return GetStringSlice(cmd, flagName)
}

// GetStringWithEnvExpandAndFallback reads a string value with environment variable expansion,
// falling back to a nested config key if the flag wasn't explicitly set
func GetStringWithEnvExpandAndFallback(cmd *cobra.Command, flagName, configKey string) (string, error) {
	var val string

	// Check if flag was explicitly set on command line
	if cmd.Flags().Changed(flagName) {
		val = GetString(cmd, flagName)
	} else if viper.IsSet(configKey) {
		// Try to get from nested config key
		val = viper.GetString(configKey)
	} else {
		// Fall back to flag default
		val = GetString(cmd, flagName)
	}

	// Expand environment variables
	val = os.ExpandEnv(val)

	isNoSensContFound, err := verifyNoSensitiveContent(val)
	if !isNoSensContFound {
		return "", fmt.Errorf("Sensitive content found in flag %v: %w", flagName, err)
	}

	return val, nil
}
