package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type configValueKind int

const (
	configString configValueKind = iota
	configBool
)

var configurableKeys = map[string]configValueKind{
	"defaults.language":  configString,
	"defaults.project":   configString,
	"defaults.style":     configString,
	"github.tokenenvvar": configString,
	"github.username":    configString,
	"llm.apikeyenvvar":   configString,
	"llm.baseurl":        configString,
	"llm.enabled":        configBool,
	"llm.model":          configString,
	"llm.provider":       configString,
	"remote.branch":      configString,
	"remote.enabled":     configBool,
	"remote.url":         configString,
	"storage.path":       configString,
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and update DevLog configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return printConfig()
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration values",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return printConfig()
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Print one configuration value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, err := canonicalConfigKey(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), viper.Get(key))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set and persist one configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, err := canonicalConfigKey(args[0])
		if err != nil {
			return err
		}
		value, err := parseConfigValue(key, args[1])
		if err != nil {
			return err
		}
		viper.Set(key, value)
		if err := persistConfig(); err != nil {
			return fmt.Errorf("could not save configuration: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s updated.\n", key)
		return nil
	},
}

func canonicalConfigKey(value string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(value))
	if _, ok := configurableKeys[key]; !ok {
		keys := make([]string, 0, len(configurableKeys))
		for candidate := range configurableKeys {
			keys = append(keys, candidate)
		}
		sort.Strings(keys)
		return "", fmt.Errorf("unknown configuration key %q; valid keys: %s", value, strings.Join(keys, ", "))
	}
	return key, nil
}

func parseConfigValue(key, value string) (any, error) {
	if configurableKeys[key] == configBool {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("invalid value for %s: expected true or false", key)
		}
		return parsed, nil
	}
	return value, nil
}

func printConfig() error {
	settings := viper.AllSettings()
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode configuration: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func init() {
	RootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configListCmd, configGetCmd, configSetCmd)
}
