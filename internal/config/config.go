// Package config provides configuration loading and management for microclaw.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// WebChannelConfig holds settings for the web channel.
type WebChannelConfig struct {
	Enabled bool `yaml:"enabled"`
}

// TelegramChannelConfig holds settings for the Telegram channel.
type TelegramChannelConfig struct {
	Enabled        bool    `yaml:"enabled"`
	BotToken       string  `yaml:"bot_token"`
	BotUsername    string  `yaml:"bot_username"`
	AllowedGroups  []int64 `yaml:"allowed_groups"`
	AllowedUserIDs []int64 `yaml:"allowed_user_ids"`
}

// DiscordChannelConfig holds settings for the Discord channel.
type DiscordChannelConfig struct {
	Enabled         bool     `yaml:"enabled"`
	BotToken        string   `yaml:"bot_token"`
	AllowedChannels []string `yaml:"allowed_channels"`
}

// SlackChannelConfig holds settings for the Slack channel.
type SlackChannelConfig struct {
	Enabled  bool   `yaml:"enabled"`
	BotToken string `yaml:"bot_token"`
	AppToken string `yaml:"app_token"`
}

// ChannelsConfig aggregates all channel configurations.
type ChannelsConfig struct {
	Web      WebChannelConfig      `yaml:"web"`
	Telegram TelegramChannelConfig `yaml:"telegram"`
	Discord  DiscordChannelConfig  `yaml:"discord"`
	Slack    SlackChannelConfig    `yaml:"slack"`
}

// Config holds the complete microclaw configuration.
type Config struct {
	LLMProvider                        string  `yaml:"llm_provider"`
	APIKey                             string  `yaml:"api_key"`
	BaseURL                            string  `yaml:"base_url"`
	Model                              string  `yaml:"model"`
	MaxTokens                          int     `yaml:"max_tokens"`
	MaxToolIterations                  int     `yaml:"max_tool_iterations"`
	MaxHistoryMessages                 int     `yaml:"max_history_messages"`
	MaxDocumentSizeMB                  int     `yaml:"max_document_size_mb"`
	MemoryTokenBudget                  int     `yaml:"memory_token_budget"`
	DataDir                            string  `yaml:"data_dir"`
	WorkingDir                         string  `yaml:"working_dir"`
	WorkingDirIsolation                string  `yaml:"working_dir_isolation"`
	HighRiskToolUserConfirmationRequired bool   `yaml:"high_risk_tool_user_confirmation_required"`
	Timezone                           string  `yaml:"timezone"`
	MaxSessionMessages                 int     `yaml:"max_session_messages"`
	CompactKeepRecent                  int     `yaml:"compact_keep_recent"`
	Channels                           ChannelsConfig `yaml:"channels"`
	WebHost                            string  `yaml:"web_host"`
	WebPort                            int     `yaml:"web_port"`
	SoulPath                           *string `yaml:"soul_path"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".microclaw")
	return &Config{
		LLMProvider:                        "anthropic",
		MaxTokens:                          8192,
		MaxToolIterations:                  100,
		MaxHistoryMessages:                 50,
		MaxDocumentSizeMB:                  100,
		MemoryTokenBudget:                  1500,
		DataDir:                            dataDir,
		WorkingDir:                         filepath.Join(dataDir, "working_dir"),
		WorkingDirIsolation:                "chat",
		HighRiskToolUserConfirmationRequired: true,
		Timezone:                           "UTC",
		MaxSessionMessages:                 40,
		CompactKeepRecent:                  20,
		WebHost:                            "127.0.0.1",
		WebPort:                            10961,
		Channels: ChannelsConfig{
			Web: WebChannelConfig{Enabled: true},
		},
	}
}

// DefaultModel returns the default model name for the given provider.
func DefaultModel(provider string) string {
	switch strings.ToLower(provider) {
	case "anthropic":
		return "claude-sonnet-4-5-20250929"
	case "openai":
		return "gpt-4o"
	case "gemini":
		return "gemini-2.5-flash"
	case "openrouter":
		return "anthropic/claude-sonnet-4-5"
	case "deepseek":
		return "deepseek-chat"
	case "ollama":
		return "llama3.2"
	default:
		return "gpt-4o"
	}
}

// BaseURLForProvider returns the API base URL for the given provider.
func BaseURLForProvider(provider string) string {
	switch strings.ToLower(provider) {
	case "anthropic":
		return "https://api.anthropic.com/v1"
	case "openai":
		return "https://api.openai.com/v1"
	case "openai-compatible":
		return ""
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "ollama":
		return "http://localhost:11434/v1"
	default:
		return "https://api.openai.com/v1"
	}
}

// Load reads config from the given file path. Missing file returns defaults.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}
	// Expand tilde in paths.
	cfg.DataDir = expandHome(cfg.DataDir)
	cfg.WorkingDir = expandHome(cfg.WorkingDir)
	// Fill model default if not set.
	if cfg.Model == "" {
		cfg.Model = DefaultModel(cfg.LLMProvider)
	}
	// Fill base URL default if not set.
	if cfg.BaseURL == "" && cfg.LLMProvider != "gemini" {
		cfg.BaseURL = BaseURLForProvider(cfg.LLMProvider)
	}
	return cfg, nil
}

// Save writes the config to path.
func Save(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// DefaultConfigPath returns the default path for the config file.
func DefaultConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".microclaw", "config.yaml")
}
