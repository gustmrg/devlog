package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"devlog/internal/collector"
	"devlog/internal/collector/claudecode"
	"devlog/internal/collector/gitlocal"
	ghsync "devlog/internal/github"
	"devlog/internal/scheduler"
	"devlog/internal/store"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure DevLog on this device, one question at a time",
	Long: `Guides you through configuring DevLog on this device. Each question shows
the current or detected value in brackets; press Enter to keep it.

Answer none to leave out an optional value, such as a tool you do not use.

It asks what the device is for, then only what that role needs: the folders
with your repositories and your commit emails on a work machine, the GitHub
account on an always-on machine, and on both the private repository that
keeps every device's activity together. It can then schedule collection,
install the MCP service, and run a test collection, asking before each.

Run it again at any time to change an answer. Every setting can also be
changed directly with devlog config set.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
			return fmt.Errorf("devlog setup asks questions in a terminal; in scripts, use devlog init and devlog config set")
		}
		return runSetup(cmd.Context(), os.Stdin, cmd.OutOrStdout(), defaultSetupActions())
	},
}

// Device roles decide which sources a device collects and what setup asks.
const (
	roleWork = iota + 1
	roleAlwaysOn
	roleBoth
)

var (
	localSources  = []string{"git", "claude-code", "opencode"}
	remoteSources = []string{"github"}
)

// setupActions are the steps of setup that act outside the configuration file.
type setupActions struct {
	githubLogin func(ctx context.Context) (login, source string, err error)
	initRemote  func(ctx context.Context, url string) error
	schedule    func(times []scheduler.Clock, args []string, envFile string) (string, error)
	installMCP  func() error
	testCollect func(ctx context.Context) []collector.Result
}

func defaultSetupActions() setupActions {
	return setupActions{
		githubLogin: func(ctx context.Context) (string, string, error) {
			tokenEnvVar := viper.GetString("github.tokenEnvVar")
			if tokenEnvVar == "" {
				tokenEnvVar = "GITHUB_TOKEN"
			}
			token, source, err := resolveGitHubToken(ctx, tokenEnvVar)
			if err != nil {
				return "", "", err
			}
			user, _, err := ghsync.NewClient(ctx, token).REST.Users.Get(ctx, "")
			if err != nil {
				return "", source, fmt.Errorf("the GitHub token was found but could not be used: %w", err)
			}
			return user.GetLogin(), source, nil
		},
		initRemote: func(ctx context.Context, url string) error {
			return initRemote(ctx, url, "main")
		},
		schedule: func(times []scheduler.Clock, args []string, envFile string) (string, error) {
			installedAt, _, err := installSchedule("collect", times, true, args, envFile)
			return installedAt, err
		},
		installMCP: func() error { return installMCPService(mcpDefaultAddress, "") },
		testCollect: func(ctx context.Context) []collector.Result {
			loc, err := collectLocation()
			if err != nil {
				return []collector.Result{{Collector: "setup", Err: err}}
			}
			collectors, err := configuredCollectors(nil)
			if err != nil {
				return []collector.Result{{Collector: "setup", Err: err}}
			}
			tickets, err := ticketPattern()
			if err != nil {
				return []collector.Result{{Collector: "setup", Err: err}}
			}
			return collector.Run(ctx, collectors, collector.DayWindow(time.Now().In(loc), loc), collector.Options{DeviceName: deviceName(), Tickets: tickets})
		},
	}
}

// errSetupInterrupted reports that input ended before setup finished.
var errSetupInterrupted = errors.New("setup was interrupted")

type prompter struct {
	in  *bufio.Reader
	out io.Writer
}

func (p *prompter) line() (string, error) {
	text, err := p.in.ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || strings.TrimSpace(text) == "") {
		return "", errSetupInterrupted
	}
	return strings.TrimSpace(text), nil
}

// ask asks until validate accepts the answer, or the current value when the
// answer is empty, and returns what validate made of it.
func (p *prompter) ask(question, current string, validate func(string) (string, error)) (string, error) {
	for {
		if current != "" {
			fmt.Fprintf(p.out, "%s [%s]: ", question, current)
		} else {
			fmt.Fprintf(p.out, "%s: ", question)
		}
		answer, err := p.line()
		if err != nil {
			return "", err
		}
		if answer == "" {
			answer = current
		}
		if validate == nil {
			return answer, nil
		}
		value, err := validate(answer)
		if err == nil {
			return value, nil
		}
		fmt.Fprintf(p.out, "  %s %s\n", color.RedString("✗"), err)
	}
}

func (p *prompter) confirm(question string, yes bool) (bool, error) {
	hint := "[y/N]"
	if yes {
		hint = "[Y/n]"
	}
	for {
		fmt.Fprintf(p.out, "%s %s ", question, hint)
		answer, err := p.line()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "":
			return yes, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		fmt.Fprintf(p.out, "  %s answer y or n\n", color.RedString("✗"))
	}
}

func (p *prompter) choose(question string, options []string, current int) (int, error) {
	fmt.Fprintln(p.out, question)
	for i, option := range options {
		fmt.Fprintf(p.out, "  %d) %s\n", i+1, option)
	}
	answer, err := p.ask("Choice", strconv.Itoa(current), func(value string) (string, error) {
		choice, err := strconv.Atoi(value)
		if err != nil || choice < 1 || choice > len(options) {
			return "", fmt.Errorf("choose a number from 1 to %d", len(options))
		}
		return value, nil
	})
	if err != nil {
		return 0, err
	}
	choice, _ := strconv.Atoi(answer)
	return choice, nil
}

func (p *prompter) done(format string, args ...any) {
	fmt.Fprintf(p.out, "  %s %s\n", color.GreenString("✔"), fmt.Sprintf(format, args...))
}

func (p *prompter) warn(format string, args ...any) {
	fmt.Fprintf(p.out, "  %s %s\n", color.YellowString("!"), fmt.Sprintf(format, args...))
}

func runSetup(ctx context.Context, in io.Reader, out io.Writer, actions setupActions) error {
	p := &prompter{in: bufio.NewReader(in), out: out}
	if _, err := store.Initialize(); err != nil {
		return fmt.Errorf("could not initialize DevLog: %w", err)
	}
	saved := false
	err := setupSteps(ctx, p, actions, &saved)
	if errors.Is(err, errSetupInterrupted) {
		fmt.Fprintln(out)
		if saved {
			return fmt.Errorf("%w; your answers were saved, so run devlog setup again to finish", errSetupInterrupted)
		}
		return fmt.Errorf("%w; no changes were saved", errSetupInterrupted)
	}
	return err
}

func setupSteps(ctx context.Context, p *prompter, actions setupActions, saved *bool) error {
	role, err := p.choose("What is this device for?", []string{
		"Work machine: collect Git, Claude Code, and opencode activity here",
		"Always-on machine: collect GitHub activity and serve it over MCP",
		"Both",
	}, currentRole())
	if err != nil {
		return err
	}
	local, remote := role != roleAlwaysOn, role != roleWork

	name, err := p.ask("Device name", currentOr("device.name", hostname()), required("a device name"))
	if err != nil {
		return err
	}
	timezone, err := p.ask("Time zone, as an IANA name (none uses the system zone)", currentOr("collect.timezone", detectTimeZone()), validateTimeZone)
	if err != nil {
		return err
	}
	pattern, err := p.ask("Ticket key pattern", currentOr("collect.ticketpattern", collector.DefaultTicketPattern), validatePattern)
	if err != nil {
		return err
	}

	settings := map[string]any{
		"device.name":           name,
		"collect.timezone":      timezone,
		"collect.ticketpattern": pattern,
	}
	var sources []string
	if local {
		sources = append(sources, localSources...)
		if err := askLocalSources(p, settings); err != nil {
			return err
		}
	}
	envFile := ""
	if remote {
		sources = append(sources, remoteSources...)
		if envFile, err = askGitHub(ctx, p, actions, settings); err != nil {
			return err
		}
	}
	settings["collect.sources"] = sources

	for key, value := range settings {
		viper.Set(key, value)
	}
	if err := persistConfig(); err != nil {
		return fmt.Errorf("could not save the configuration: %w", err)
	}
	*saved = true
	p.done("configuration saved")

	if err := askRemote(ctx, p, actions); err != nil {
		return err
	}
	if err := askSchedule(p, actions, role, envFile); err != nil {
		return err
	}
	if remote {
		if err := askMCP(p, actions); err != nil {
			return err
		}
	}
	if ok, err := p.confirm("Run a test collection now? Nothing is saved", true); err != nil {
		return err
	} else if ok {
		fmt.Fprintln(p.out, "  collecting today's activity…")
		reportTestCollection(p, actions.testCollect(ctx))
	}
	fmt.Fprintln(p.out, "Setup complete. Run devlog setup again or use devlog config set to change any answer.")
	return nil
}

func askLocalSources(p *prompter, settings map[string]any) error {
	roots, err := p.ask("Folders with your repositories (comma-separated)", strings.Join(configList("collect.git.roots"), ", "), validateRoots)
	if err != nil {
		return err
	}
	rootList := splitList(roots)
	repositories, _ := gitlocal.Discover(rootList, 0)
	if len(repositories) == 0 {
		p.warn("no Git repositories found yet under these folders")
	} else {
		p.done("found %s", plural(len(repositories), "repository", "repositories"))
	}
	settings["collect.git.roots"] = rootList

	suggested := configList("collect.git.authoremails")
	if len(suggested) == 0 {
		suggested = commitEmails(repositories)
	}
	emails, err := p.ask("Your commit emails, comma-separated (none uses each repository's user.email)", strings.Join(suggested, ", "), optional(validateEmails))
	if err != nil {
		return err
	}
	settings["collect.git.authoremails"] = splitList(emails)

	opencode := viper.GetString("collect.opencode.command")
	if opencode == "" {
		if path, err := exec.LookPath("opencode"); err == nil {
			opencode, _ = filepath.Abs(path)
		}
	}
	command, err := p.ask("opencode executable, kept as a full path so scheduled runs find it (none if you do not use opencode)", opencode, validateExecutable)
	if err != nil {
		return err
	}
	settings["collect.opencode.command"] = command

	transcripts := viper.GetString("collect.claudecode.path")
	if transcripts == "" {
		if dir, err := claudecode.DefaultDir(); err == nil {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				p.done("Claude Code transcripts found in %s", dir)
				return nil
			}
		}
	}
	transcripts, err = p.ask("Claude Code transcripts folder (none if you do not use Claude Code)", transcripts, validateOptionalDir)
	if err != nil {
		return err
	}
	settings["collect.claudecode.path"] = transcripts
	return nil
}

// askGitHub checks the GitHub token and asks for the account to collect. It
// returns the environment file scheduled runs should load, if any.
func askGitHub(ctx context.Context, p *prompter, actions setupActions, settings map[string]any) (string, error) {
	login, source, loginErr := actions.githubLogin(ctx)
	switch {
	case loginErr != nil:
		p.warn("%s", loginErr)
		p.warn("collection from GitHub needs a token: set GITHUB_TOKEN or run gh auth login, then run devlog setup again")
	case source == "gh":
		p.done("GitHub CLI is signed in as %s", login)
	default:
		p.done("GitHub token found for %s", login)
	}
	username, err := p.ask("GitHub username to collect", currentOr("github.username", login), required("a GitHub username"))
	if err != nil {
		return "", err
	}
	settings["github.username"] = username
	if loginErr != nil || source != "environment" {
		return "", nil
	}
	// Scheduled jobs do not see variables set in an interactive shell.
	envFile, err := defaultEnvFile()
	if err != nil {
		return "", err
	}
	keep, err := p.confirm(fmt.Sprintf("Scheduled runs will not see your shell's token. Save it in %s, readable only by you?", envFile), true)
	if err != nil || !keep {
		return "", err
	}
	tokenEnvVar := viper.GetString("github.tokenEnvVar")
	if tokenEnvVar == "" {
		tokenEnvVar = "GITHUB_TOKEN"
	}
	if err := saveEnvValue(envFile, tokenEnvVar, os.Getenv(tokenEnvVar)); err != nil {
		return "", err
	}
	p.done("saved %s in %s", tokenEnvVar, envFile)
	return envFile, nil
}

func askRemote(ctx context.Context, p *prompter, actions setupActions) error {
	if viper.GetBool("remote.enabled") {
		p.done("syncing with %s", viper.GetString("remote.url"))
		return nil
	}
	for {
		url, err := p.ask("Private Git repository for your DevLog data, such as git@github.com:you/devlog-data.git (none to skip)", "", optional(nil))
		if err != nil || url == "" {
			return err
		}
		if err := actions.initRemote(ctx, url); err != nil {
			p.warn("%s", err)
			continue
		}
		p.done("connected to %s", url)
		return nil
	}
}

func askSchedule(p *prompter, actions setupActions, role int, envFile string) error {
	defaults := "12:15,18:15"
	if role == roleAlwaysOn {
		defaults = "12:20,18:20"
	}
	answer, err := p.ask("Collect automatically on weekdays at these times (none to skip)", defaults, optional(func(value string) (string, error) {
		_, err := scheduler.ParseTimes(value)
		return value, err
	}))
	if err != nil || answer == "" {
		return err
	}
	times, _ := scheduler.ParseTimes(answer)
	args := []string{"collect"}
	if viper.GetBool("remote.enabled") {
		args = append(args, "--remote")
	}
	installedAt, err := actions.schedule(times, args, envFile)
	if err != nil {
		p.warn("could not schedule collection: %s", err)
		return nil
	}
	p.done("scheduled (%s)", installedAt)
	return nil
}

// setupGOOS is the platform setup offers services for; tests replace it.
var setupGOOS = runtime.GOOS

func askMCP(p *prompter, actions setupActions) error {
	if setupGOOS != "linux" {
		p.warn("to serve activity to agents, run devlog mcp --http %s under your service manager", mcpDefaultAddress)
		return nil
	}
	install, err := p.confirm(fmt.Sprintf("Install the MCP server as a service on %s?", mcpDefaultAddress), true)
	if err != nil || !install {
		return err
	}
	if err := actions.installMCP(); err != nil {
		p.warn("could not install the MCP service: %s", err)
		return nil
	}
	p.done("to reach it from other devices over HTTPS: tailscale serve --bg --https=8444 http://%s", mcpDefaultAddress)
	return nil
}

func reportTestCollection(p *prompter, results []collector.Result) {
	for _, result := range results {
		switch {
		case result.Err != nil:
			p.warn("%s failed: %s", result.Collector, result.Err)
		case result.Skipped != "":
			p.warn("%s skipped: %s", result.Collector, result.Skipped)
		default:
			p.done("%s: %s today", result.Collector, plural(len(result.Entries), "entry", "entries"))
		}
	}
}

// currentRole guesses the role from the configured sources.
func currentRole() int {
	sources := configList("collect.sources")
	hasRemote := slices.Contains(sources, "github")
	hasLocal := slices.ContainsFunc(sources, func(source string) bool { return slices.Contains(localSources, source) })
	switch {
	case hasRemote && hasLocal:
		return roleBoth
	case hasRemote:
		return roleAlwaysOn
	default:
		return roleWork
	}
}

func currentOr(key, fallback string) string {
	if value := strings.TrimSpace(viper.GetString(key)); value != "" {
		return value
	}
	return fallback
}

func hostname() string {
	host, _ := os.Hostname()
	return host
}

// detectTimeZone returns the system's IANA zone name when it can tell.
func detectTimeZone() string {
	if name := strings.TrimPrefix(os.Getenv("TZ"), ":"); name != "" {
		if _, err := time.LoadLocation(name); err == nil {
			return name
		}
	}
	if target, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
		if _, name, ok := strings.Cut(filepath.ToSlash(target), "zoneinfo/"); ok {
			if _, err := time.LoadLocation(name); err == nil {
				return name
			}
		}
	}
	return ""
}

func required(what string) func(string) (string, error) {
	return func(value string) (string, error) {
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("enter %s", what)
		}
		return strings.TrimSpace(value), nil
	}
}

// optional lets "none" clear an answer and checks any other with validate.
func optional(validate func(string) (string, error)) func(string) (string, error) {
	return func(value string) (string, error) {
		if value == "" || strings.EqualFold(value, "none") {
			return "", nil
		}
		if validate == nil {
			return value, nil
		}
		return validate(value)
	}
}

func validateTimeZone(value string) (string, error) {
	if value == "" || strings.EqualFold(value, "none") {
		return "", nil
	}
	if _, err := time.LoadLocation(value); err != nil {
		return "", fmt.Errorf("unknown time zone %q; use an IANA name such as Europe/Lisbon", value)
	}
	return value, nil
}

func validatePattern(value string) (string, error) {
	if value == "" {
		return collector.DefaultTicketPattern, nil
	}
	if _, err := regexp.Compile(value); err != nil {
		return "", fmt.Errorf("invalid regular expression: %w", err)
	}
	return value, nil
}

func validateRoots(value string) (string, error) {
	roots := splitList(value)
	if len(roots) == 0 {
		return "", fmt.Errorf("enter at least one folder")
	}
	for _, root := range roots {
		if err := validateDir(root); err != nil {
			return "", err
		}
	}
	return strings.Join(roots, ", "), nil
}

func validateOptionalDir(value string) (string, error) {
	if value == "" || strings.EqualFold(value, "none") {
		return "", nil
	}
	return value, validateDir(value)
}

func validateDir(path string) error {
	expanded, err := absolutePath(path)
	if err != nil {
		return err
	}
	if info, err := os.Stat(expanded); err != nil || !info.IsDir() {
		return fmt.Errorf("%s is not a folder", path)
	}
	return nil
}

func validateEmails(value string) (string, error) {
	emails := splitList(value)
	for _, email := range emails {
		if _, err := mail.ParseAddress(email); err != nil || strings.ContainsAny(email, "<> ") {
			return "", fmt.Errorf("%s is not an email address", email)
		}
	}
	return strings.Join(emails, ", "), nil
}

func validateExecutable(value string) (string, error) {
	if value == "" || strings.EqualFold(value, "none") {
		return "", nil
	}
	path, err := exec.LookPath(value)
	if err != nil {
		return "", fmt.Errorf("%s is not an executable", value)
	}
	return filepath.Abs(path)
}

func plural(count int, singular, many string) string {
	if count == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(count) + " " + many
}

func splitList(value string) []string {
	var items []string
	for item := range strings.SplitSeq(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" && !slices.Contains(items, trimmed) {
			items = append(items, trimmed)
		}
	}
	return items
}

// commitEmails lists the user.email Git uses in each repository, global
// settings included.
func commitEmails(repositories []string) []string {
	var emails []string
	for _, dir := range repositories {
		output, err := exec.Command("git", "-C", dir, "config", "user.email").Output()
		if email := strings.TrimSpace(string(output)); err == nil && email != "" && !slices.Contains(emails, email) {
			emails = append(emails, email)
		}
	}
	return emails
}

func defaultEnvFile() (string, error) {
	root, err := store.ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "collect.env"), nil
}

// saveEnvValue sets key in a KEY=VALUE file readable only by its owner,
// keeping its other lines.
func saveEnvValue(path, key, value string) error {
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		for line := range strings.SplitSeq(strings.TrimRight(string(data), "\n"), "\n") {
			name, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "export "), "=")
			if strings.TrimSpace(name) != key {
				lines = append(lines, line)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("could not read %s: %w", path, err)
	}
	lines = append(lines, key+"="+value)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("could not write %s: %w", path, err)
	}
	return os.Chmod(path, 0o600)
}

func init() {
	RootCmd.AddCommand(setupCmd)
}
