// Command microclaw is an agentic AI assistant for chat surfaces.
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"

	"github.com/linkerlin/microclaw.go/internal/config"
	"github.com/linkerlin/microclaw.go/internal/runtime"
)

var (
	version    = "0.1.0"
	configPath string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "microclaw",
	Short: "MicroClaw - Agentic AI assistant for chat surfaces",
	Long:  `MicroClaw is a Go implementation of an agentic AI assistant supporting multiple chat platforms and LLM providers.`,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", config.DefaultConfigPath(), "config file path")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(webCmd)
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the microclaw runtime",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		rt, err := runtime.New(cfg)
		if err != nil {
			return fmt.Errorf("initializing runtime: %w", err)
		}

		log.Printf("Starting MicroClaw v%s (provider: %s, model: %s)", version, cfg.LLMProvider, cfg.Model)
		return rt.Run(context.Background())
	},
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup wizard",
	RunE: func(cmd *cobra.Command, args []string) error {
		scanner := bufio.NewScanner(os.Stdin)

		cfg, err := config.Load(configPath)
		if err != nil {
			cfg = config.DefaultConfig()
		}

		fmt.Println("=== MicroClaw Setup Wizard ===")
		fmt.Println()

		cfg.LLMProvider = prompt(scanner, "LLM Provider (anthropic/openai/gemini/openrouter/deepseek/ollama)", cfg.LLMProvider)
		cfg.APIKey = prompt(scanner, "API Key", cfg.APIKey)

		defaultModel := config.DefaultModel(cfg.LLMProvider)
		cfg.Model = prompt(scanner, fmt.Sprintf("Model (default: %s)", defaultModel), defaultModel)
		if cfg.Model == "" {
			cfg.Model = defaultModel
		}

		enableWebStr := "y"
		if !cfg.Channels.Web.Enabled {
			enableWebStr = "n"
		}
		webEnabled := prompt(scanner, "Enable web channel? (y/n)", enableWebStr)
		cfg.Channels.Web.Enabled = strings.ToLower(webEnabled) == "y"

		enableTg := prompt(scanner, "Enable Telegram channel? (y/n)", "n")
		cfg.Channels.Telegram.Enabled = strings.ToLower(enableTg) == "y"
		if cfg.Channels.Telegram.Enabled {
			cfg.Channels.Telegram.BotToken = prompt(scanner, "Telegram Bot Token", cfg.Channels.Telegram.BotToken)
		}

		if err := config.Save(cfg, configPath); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		fmt.Printf("\nConfig saved to: %s\n", configPath)
		fmt.Println("Run 'microclaw start' to launch.")
		return nil
	},
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run preflight diagnostics",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			fmt.Printf("❌ Config: %v\n", err)
			return nil
		}
		fmt.Printf("✅ Config loaded from: %s\n", configPath)
		fmt.Printf("   Provider: %s\n", cfg.LLMProvider)
		fmt.Printf("   Model: %s\n", cfg.Model)
		fmt.Printf("   Data dir: %s\n", cfg.DataDir)

		if cfg.APIKey == "" && cfg.LLMProvider != "ollama" {
			fmt.Printf("⚠️  API key not set\n")
		} else if cfg.LLMProvider != "ollama" {
			fmt.Printf("✅ API key is set\n")
		}

		if cfg.Channels.Web.Enabled {
			fmt.Printf("✅ Web channel enabled on %s:%d\n", cfg.WebHost, cfg.WebPort)
		}
		if cfg.Channels.Telegram.Enabled {
			if cfg.Channels.Telegram.BotToken == "" {
				fmt.Printf("❌ Telegram enabled but bot_token not set\n")
			} else {
				fmt.Printf("✅ Telegram channel enabled\n")
			}
		}

		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("microclaw v%s\n", version)
	},
}

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Web channel management",
}

var webPasswordCmd = &cobra.Command{
	Use:   "password <value>",
	Short: "Set the web UI password",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		rt, err := runtime.New(cfg)
		if err != nil {
			return fmt.Errorf("initializing runtime: %w", err)
		}

		// Hash the password with bcrypt before storing.
		hash, err := bcrypt.GenerateFromPassword([]byte(args[0]), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hashing password: %w", err)
		}
		if err := rt.DB().SetWebPassword(string(hash)); err != nil {
			return fmt.Errorf("setting password: %w", err)
		}

		fmt.Println("Web UI password updated.")
		return nil
	},
}

func init() {
	webCmd.AddCommand(webPasswordCmd)
}

func prompt(scanner *bufio.Scanner, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	scanner.Scan()
	val := strings.TrimSpace(scanner.Text())
	if val == "" {
		return defaultVal
	}
	return val
}
