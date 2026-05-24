package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

func skillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "List and manage skills",
	}
	cmd.AddCommand(skillsListCmd())
	cmd.AddCommand(skillsShowCmd())
	return cmd
}

func skillsListCmd() *cobra.Command {
	var jsonOutput bool
	var agentID string
	var localOnly bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all available skills",
		Run: func(cmd *cobra.Command, args []string) {
			// --agent: per-agent view via gateway
			if agentID != "" && isGatewayReachable() {
				runSkillsListHTTP(agentID, jsonOutput)
				return
			}
			// Default when gateway is reachable: list DB-backed tenant skills
			// (catches gcplane-uploaded skills like gh-read that aren't on disk).
			// --local forces the filesystem-only fallback.
			if !localOnly && isGatewayReachable() {
				runSkillsListTenantHTTP(jsonOutput)
				return
			}

			// Fallback: filesystem-based skill listing
			loader := loadSkillsLoader()
			allSkills := loader.ListSkills(context.Background())

			if jsonOutput {
				data, _ := json.MarshalIndent(allSkills, "", "  ")
				fmt.Println(string(data))
				return
			}

			if len(allSkills) == 0 {
				fmt.Println("No skills found.")
				return
			}

			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(tw, "NAME\tSOURCE\tDESCRIPTION\n")
			for _, s := range allSkills {
				desc := s.Description
				if runes := []rune(desc); len(runes) > 60 {
					desc = string(runes[:57]) + "..."
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Name, s.Source, desc)
			}
			tw.Flush()
		},
	}
	cmd.Flags().StringVar(&agentID, "agent", "", "agent ID to list skills for (uses gateway API)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&localOnly, "local", false, "force filesystem-only listing (skip gateway DB lookup)")
	return cmd
}

func skillsShowCmd() *cobra.Command {
	var localOnly bool
	cmd := &cobra.Command{
		Use:   "show [name]",
		Short: "Show details and content of a skill",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Default: try DB-backed skill via gateway first (catches
			// gcplane-uploaded skills like gh-read). --local forces filesystem.
			if !localOnly && isGatewayReachable() {
				if runSkillsShowHTTP(args[0]) {
					return
				}
				// fall through to filesystem if HTTP didn't find it
			}
			loader := loadSkillsLoader()
			info, ok := loader.GetSkill(context.Background(), args[0])
			if !ok {
				fmt.Fprintf(os.Stderr, "Skill not found: %s\n", args[0])
				os.Exit(1)
			}
			fmt.Printf("Name:        %s\n", info.Name)
			fmt.Printf("Description: %s\n", info.Description)
			fmt.Printf("Source:      %s\n", info.Source)
			fmt.Printf("Location:    %s\n", info.Path)
			fmt.Println()

			content, ok := loader.LoadSkill(context.Background(), args[0])
			if ok {
				fmt.Println("--- Content ---")
				fmt.Println(content)
			}
		},
	}
	cmd.Flags().BoolVar(&localOnly, "local", false, "force filesystem-only lookup (skip gateway DB lookup)")
	return cmd
}

// runSkillsShowHTTP looks up a skill by slug via /v1/skills.
// Returns true if found + printed; false if not found (caller falls back to FS).
func runSkillsShowHTTP(slug string) bool {
	resp, err := gatewayHTTPGet("/v1/skills")
	if err != nil {
		return false
	}
	raw, _ := json.Marshal(resp["skills"])
	var skills []map[string]any
	if err := json.Unmarshal(raw, &skills); err != nil {
		return false
	}
	for _, s := range skills {
		if str(s["slug"]) != slug {
			continue
		}
		fmt.Printf("Slug:        %s\n", str(s["slug"]))
		fmt.Printf("Name:        %s\n", str(s["name"]))
		fmt.Printf("Description: %s\n", str(s["description"]))
		fmt.Printf("Source:      %s\n", str(s["source"]))
		if v := str(s["version"]); v != "" {
			fmt.Printf("Version:     %s\n", v)
		}
		if v := str(s["visibility"]); v != "" {
			fmt.Printf("Visibility:  %s\n", v)
		}
		return true
	}
	return false
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// runSkillsListTenantHTTP fetches the caller's tenant skills via /v1/skills.
// Covers DB-backed skills (gcplane upload, manual POST /v1/skills/upload) that
// the filesystem loader misses since they live in the skills-store + DB row.
func runSkillsListTenantHTTP(jsonOutput bool) {
	resp, err := gatewayHTTPGet("/v1/skills")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if jsonOutput {
		data, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(data))
		return
	}
	raw, _ := json.Marshal(resp["skills"])
	var skills []struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Source      string `json:"source"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &skills); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing skills: %v\n", err)
		os.Exit(1)
	}
	if len(skills) == 0 {
		fmt.Println("No skills found.")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "SLUG\tSOURCE\tDESCRIPTION\n")
	for _, s := range skills {
		desc := s.Description
		if runes := []rune(desc); len(runes) > 60 {
			desc = string(runes[:57]) + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Slug, s.Source, desc)
	}
	tw.Flush()
}

// runSkillsListHTTP fetches skills for a specific agent from the gateway API.
func runSkillsListHTTP(agentID string, jsonOutput bool) {
	resp, err := gatewayHTTPGet("/v1/agents/" + url.PathEscape(agentID) + "/skills")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(data))
		return
	}

	raw, _ := json.Marshal(resp["skills"])
	var skills []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &skills); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing skills: %v\n", err)
		os.Exit(1)
	}

	if len(skills) == 0 {
		fmt.Println("No skills found for this agent.")
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "NAME\tDESCRIPTION\n")
	for _, s := range skills {
		desc := s.Description
		if runes := []rune(desc); len(runes) > 60 {
			desc = string(runes[:57]) + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\n", s.Name, desc)
	}
	tw.Flush()
}

func loadSkillsLoader() *skills.Loader {
	cfgPath := resolveConfigPath()
	cfg, _ := config.Load(cfgPath)
	workspace := config.ExpandHome(cfg.Agents.Defaults.Workspace)
	globalSkillsDir := os.Getenv("GOCLAW_SKILLS_DIR")
	if globalSkillsDir == "" {
		globalSkillsDir = filepath.Join(cfg.ResolvedDataDir(), "skills")
	}
	builtinSkillsDir := os.Getenv("GOCLAW_BUILTIN_SKILLS_DIR")
	if builtinSkillsDir == "" {
		builtinSkillsDir = "/app/bundled-skills"
	}
	return skills.NewLoader(workspace, globalSkillsDir, builtinSkillsDir)
}
