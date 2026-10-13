package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/vibexp/vibexp/internal/auth/password"
	"github.com/vibexp/vibexp/internal/config"
	"github.com/vibexp/vibexp/internal/database"
	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
	"github.com/vibexp/vibexp/internal/repositories/postgres"
	"github.com/vibexp/vibexp/internal/services"
)

// adminCommandTimeout bounds one `vibexp admin` command: a handful of
// primary-key reads and at most one audited write.
const adminCommandTimeout = 30 * time.Second

// authRecovery is the slice of services.AuthRecoveryService the commands use,
// so the command tree is testable without a database.
type authRecovery interface {
	ListProviders(ctx context.Context) ([]*models.InstanceAuthProvider, error)
	SetProviderEnabled(ctx context.Context, slug string, enabled bool) (services.ProviderToggle, error)
	ClearAllowlist(ctx context.Context) (bool, error)
	RearmSetup(ctx context.Context) (string, error)
	SetRootAdminPassword(ctx context.Context, email, plaintext string, enableLogin bool,
	) (*services.RootAdminPasswordResult, error)
}

// authRecoveryOpener connects a command to the instance's auth settings. It
// returns the operations and a function releasing whatever it opened.
type authRecoveryOpener func(ctx context.Context) (authRecovery, func(), error)

func init() {
	rootCmd.AddCommand(newAdminCmd(openAuthRecovery))
}

// newAdminCmd builds the `vibexp admin` tree (#1237): break-glass commands that
// edit the instance's settings directly in the database, with no running
// server, for an operator who can no longer sign in. They suit `docker exec`.
func newAdminCmd(open authRecoveryOpener) *cobra.Command {
	admin := groupCmd("admin", "Administer the instance directly in the database, without a running server")
	auth := groupCmd("auth", "Recover from an authentication lockout")
	auth.Long = `Recover from an authentication misconfiguration that locks everyone out.

These commands load the same config.yaml as the server and write straight to
the database, so they work when nobody can sign in. A running server picks a
change up within seconds; no restart is needed. Every change is recorded in the
instance settings audit log with no user and source "cli".

They never run database migrations: start the server once after an upgrade.`

	providers := groupCmd("providers", "List, enable or disable sign-in identity providers")
	providers.AddCommand(
		open.command("list", "List the stored identity providers", cobra.NoArgs, runProvidersList),
		open.command("disable <slug>", "Stop offering an identity provider at sign-in", cobra.ExactArgs(1),
			runProviderToggle(false)),
		open.command("enable <slug>", "Offer an identity provider at sign-in again", cobra.ExactArgs(1),
			runProviderToggle(true)),
	)

	allowlist := groupCmd("allowlist", "Manage the sign-in access allowlist")
	allowlist.AddCommand(
		open.command("clear", "Remove the access allowlist, reverting to open access", cobra.NoArgs,
			runAllowlistClear),
	)

	setup := groupCmd("setup", "Manage authentication setup mode")
	setup.AddCommand(
		open.command("rearm", "Force setup mode on and print a new one-time setup URL", cobra.NoArgs,
			runSetupRearm),
	)

	passwords := groupCmd("password", "Set a root instance admin's password")
	passwords.AddCommand(newPasswordSetCmd(open))

	auth.AddCommand(providers, allowlist, setup, passwords)
	admin.AddCommand(auth)
	return admin
}

// groupCmd is a command that only groups subcommands: alone it prints its help,
// and an unknown subcommand is an error rather than a silent help page.
func groupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
}

// authRecoveryRun is the body of one leaf command.
type authRecoveryRun func(ctx context.Context, recovery authRecovery, out io.Writer, args []string) error

// command builds a leaf command that opens the auth settings, runs run under a
// bounded context and releases what it opened. A failure is reported once, by
// Execute, without the usage text: nothing here fails for a usage reason once
// the arguments are validated.
func (open authRecoveryOpener) command(
	use, short string, args cobra.PositionalArgs, run authRecoveryRun,
) *cobra.Command {
	return &cobra.Command{
		Use:           use,
		Short:         short,
		Args:          args,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), adminCommandTimeout)
			defer cancel()
			recovery, release, err := open(ctx)
			if err != nil {
				return err
			}
			defer release()
			return run(ctx, recovery, cmd.OutOrStdout(), args)
		},
	}
}

func runProvidersList(ctx context.Context, recovery authRecovery, out io.Writer, _ []string) error {
	providers, err := recovery.ListProviders(ctx)
	if err != nil {
		return err
	}
	if len(providers) == 0 {
		_, err := fmt.Fprintln(out, "No identity providers are stored.")
		return err
	}
	// Only whether a client secret is stored is shown, never the ciphertext.
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "SLUG\tTYPE\tENABLED\tDISPLAY NAME\tCLIENT SECRET"); err != nil {
		return err
	}
	for _, p := range providers {
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n",
			p.Slug, p.Type, yesNo(p.Enabled), p.DisplayName, storedOrNone(p.HasClientSecret())); err != nil {
			return err
		}
	}
	return table.Flush()
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func storedOrNone(stored bool) string {
	if stored {
		return "stored"
	}
	return "none"
}

// runProviderToggle is the body of `providers enable` and `providers disable`.
func runProviderToggle(enabled bool) authRecoveryRun {
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	return func(ctx context.Context, recovery authRecovery, out io.Writer, args []string) error {
		slug := args[0]
		result, err := recovery.SetProviderEnabled(ctx, slug, enabled)
		if errors.Is(err, repositories.ErrInstanceAuthProviderNotFound) {
			return fmt.Errorf("no identity provider has the slug %q; "+
				"run `vibexp admin auth providers list` to see the stored ones", slug)
		}
		if err != nil {
			return err
		}
		if !result.Changed {
			_, err = fmt.Fprintf(out, "Identity provider %q is already %s; no change.\n", slug, state)
			return err
		}
		if _, err = fmt.Fprintf(out, "Identity provider %q %s.\n", slug, state); err != nil {
			return err
		}
		switch {
		case result.NoneEnabled && result.PasswordLoginWorks:
			// Not setup mode: re-arming here would only hand out a setup URL
			// that can replace a root admin's password.
			_, err = fmt.Fprintln(out, "No identity provider is enabled now. Email and password sign-in still works, "+
				"so disabling it does not put the instance in setup mode.")
		case result.NoneEnabled:
			_, err = fmt.Fprintln(out, "No sign-in method works now, so the instance is in setup mode. "+
				"Run `vibexp admin auth setup rearm` for a setup URL.")
		}
		return err
	}
}

func runAllowlistClear(ctx context.Context, recovery authRecovery, out io.Writer, _ []string) error {
	cleared, err := recovery.ClearAllowlist(ctx)
	if err != nil {
		return err
	}
	if !cleared {
		_, err = fmt.Fprintln(out, "No access allowlist is stored; no change.")
		return err
	}
	_, err = fmt.Fprintln(out, "Access allowlist cleared: sign-in is open to every account the identity providers accept.")
	return err
}

func runSetupRearm(ctx context.Context, recovery authRecovery, out io.Writer, _ []string) error {
	setupURL, err := recovery.RearmSetup(ctx)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Authentication setup re-armed. Any earlier setup URL and setup session no longer work.\n"+
		"Open this URL within %d hours to configure sign-in:\n%s\n",
		int(services.SetupTokenLifetime.Hours()), setupURL)
	return err
}

// passwordInputLimit bounds what `password set` reads from stdin. It is above
// the longest password the policy accepts, so an over-long one is still refused
// by the policy with its own message.
const passwordInputLimit = 4096

// secretReader reads a password without it ever being an argument. It is a
// variable so tests can replace the terminal.
var secretReader = readSecret

// newPasswordSetCmd builds `vibexp admin auth password set` (#1438): the way
// back into a password-only instance nobody can sign in to.
//
// The password is never a flag or an argument (both end up in the shell
// history and the process list): it is read from stdin when stdin is not a
// terminal, and otherwise asked for twice with echo off. It is never printed
// or logged.
func newPasswordSetCmd(open authRecoveryOpener) *cobra.Command {
	var email string
	var enableLogin bool
	cmd := &cobra.Command{
		Use:   "set --email <root-admin-email>",
		Short: "Set the password of a root instance admin (auth.instance_admins)",
		Long: `Set the password of a root instance admin, to recover a password-only instance
nobody can sign in to.

The email must be listed in auth.instance_admins and its account must exist.
The password is read from standard input when that is not a terminal:

  printf '%s' "$NEW_PASSWORD" | vibexp admin auth password set --email admin@example.com

Otherwise it is asked for twice, without echo. It must satisfy the password
policy (at least 12 characters, not a common password, not the email address).
It cannot be passed as a flag or an argument.

The account's existing sessions are ended; its API keys are kept. With
--enable-login, email and password sign-in is also switched on for the
instance.`,
		// Not cobra.NoArgs: its error quotes the argument, and the argument
		// someone passes here is most likely the password.
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return errors.New("this command takes no arguments: the password is read from standard input " +
					"or asked for, never taken from the command line")
			}
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Read the password before the bounded context starts: a person
			// typing must not race the command's timeout.
			plaintext, err := secretReader(cmd.InOrStdin(), cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), adminCommandTimeout)
			defer cancel()
			recovery, release, err := open(ctx)
			if err != nil {
				return err
			}
			defer release()
			return runPasswordSet(ctx, recovery, cmd.OutOrStdout(), email, plaintext, enableLogin)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email of the root instance admin (required)")
	cmd.Flags().BoolVar(&enableLogin, "enable-login", false, "also switch email and password sign-in on")
	if err := cmd.MarkFlagRequired("email"); err != nil {
		panic(err) // the flag is defined two lines up
	}
	return cmd
}

// readSecret returns the password from in: asked for twice without echo when in
// is a terminal, read whole otherwise. One trailing line break is dropped, so
// `echo` and a here-string work; nothing else is trimmed, because a password
// may begin or end with a space.
func readSecret(in io.Reader, prompt io.Writer) (string, error) {
	if file, ok := in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		return promptSecret(file, prompt)
	}
	// Said before reading: with an input that never closes (`docker exec -i`
	// and no pipe) the command would otherwise wait without a word.
	if _, err := fmt.Fprintln(prompt, "Reading the password from standard input..."); err != nil {
		return "", err
	}
	raw, err := io.ReadAll(io.LimitReader(in, passwordInputLimit+1))
	if err != nil {
		return "", fmt.Errorf("failed to read the password from standard input: %w", err)
	}
	if len(raw) > passwordInputLimit {
		return "", errors.New("the password on standard input is too long")
	}
	plaintext := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if plaintext == "" {
		return "", errors.New("no password on standard input; pipe one in, or run the command in a terminal to be asked")
	}
	return plaintext, nil
}

// promptSecret asks for the password twice on the terminal, with echo off.
func promptSecret(terminal *os.File, prompt io.Writer) (string, error) {
	ask := func(label string) (string, error) {
		if _, err := fmt.Fprint(prompt, label); err != nil {
			return "", err
		}
		raw, err := term.ReadPassword(int(terminal.Fd()))
		if _, nlErr := fmt.Fprintln(prompt); err == nil {
			err = nlErr
		}
		if err != nil {
			return "", fmt.Errorf("failed to read the password: %w", err)
		}
		return string(raw), nil
	}
	first, err := ask("New password: ")
	if err != nil {
		return "", err
	}
	second, err := ask("Repeat the password: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", errors.New("the two passwords do not match; nothing was changed")
	}
	if first == "" {
		return "", errors.New("the password is empty; nothing was changed")
	}
	return first, nil
}

func runPasswordSet(
	ctx context.Context, recovery authRecovery, out io.Writer, email, plaintext string, enableLogin bool,
) error {
	result, err := recovery.SetRootAdminPassword(ctx, email, plaintext, enableLogin)
	var policyErr *password.PolicyError
	switch {
	case errors.Is(err, services.ErrNotRootAdmin):
		return fmt.Errorf("%q is not a root instance admin: only an email listed in auth.instance_admins "+
			"can be given a password this way; nothing was changed", email)
	case errors.Is(err, services.ErrRootAdminAccountMissing):
		return fmt.Errorf("no account exists for %q yet, and this command cannot create one; "+
			"run `vibexp admin auth setup rearm` and create the account from the setup page; nothing was changed", email)
	case errors.Is(err, services.ErrRootAdminSuspended):
		return fmt.Errorf("the account of %q is suspended, so a password would not let it sign in; "+
			"nothing was changed", email)
	case errors.As(err, &policyErr):
		return fmt.Errorf("the password does not meet the password policy: %s Nothing was changed",
			policyViolations(policyErr))
	case err != nil:
		return err
	}

	action := "set"
	if result.PasswordReplaced {
		action = "replaced"
	}
	if _, err = fmt.Fprintf(out, "Password %s for %s. The account's existing sessions are ended.\n",
		action, result.Email); err != nil {
		return err
	}
	switch {
	case result.LoginSwitchedOn:
		_, err = fmt.Fprintln(out, "Email and password sign-in is now switched on.")
	case enableLogin:
		_, err = fmt.Fprintln(out, "Email and password sign-in was already switched on.")
	default:
		_, err = fmt.Fprintln(out, "Email and password sign-in was not changed: "+
			"if it is switched off, run the command again with --enable-login.")
	}
	return err
}

// policyViolations lists what a refused password broke, as sentences.
func policyViolations(err *password.PolicyError) string {
	messages := make([]string, 0, len(err.Violations))
	for _, violation := range err.Violations {
		messages = append(messages, violation.Message())
	}
	return strings.Join(messages, " ")
}

// openAuthRecovery is the production authRecoveryOpener: it loads the same
// config.yaml as the server and connects straight to the database. It never
// runs migrations — a break-glass command migrating a live instance's schema
// would be a surprise — and refuses to act on a database that lacks the auth
// settings tables.
func openAuthRecovery(ctx context.Context) (authRecovery, func(), error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load configuration: %w", err)
	}
	db, err := database.NewConnection(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to the database: %w", err)
	}
	if err := requireAuthSettingsTables(ctx, db); err != nil {
		closeQuietly(db)
		return nil, nil, err
	}
	return newAuthRecoveryService(db, cfg), func() { closeQuietly(db) }, nil
}

// closeQuietly closes db. The command's outcome is already decided by the time
// it runs, so a close error is only worth a warning.
func closeQuietly(db *database.DB) {
	if err := db.Close(); err != nil {
		slog.Warn("Failed to close database connection", "error", err)
	}
}

// newAuthRecoveryService builds the recovery service over the PostgreSQL
// repositories the server uses.
func newAuthRecoveryService(db *database.DB, cfg *config.Config) *services.AuthRecoveryService {
	return services.NewAuthRecoveryService(services.AuthRecoveryDeps{
		Providers:     postgres.NewInstanceAuthProviderRepository(db),
		Allowlist:     postgres.NewInstanceAuthAllowlistRepository(db),
		Setup:         postgres.NewInstanceAuthSetupRepository(db),
		Version:       postgres.NewInstanceAuthSettingsVersionRepository(db),
		WayIn:         newAuthWayIn(db, cfg),
		RootPasswords: newRootAdminPasswordService(db, cfg),
		BaseURL:       cfg.Frontend.BaseURL,
	})
}

// newAuthWayIn builds the "does a sign-in method work" check the server uses,
// over the database and the configured root admins.
func newAuthWayIn(db *database.DB, cfg *config.Config) services.AuthWayIn {
	admins := services.NewInstanceAdminService(cfg.Auth.InstanceAdmins, nil, nil, slog.New(slog.DiscardHandler))
	return services.NewAuthWayIn(services.AuthWayInDeps{
		Providers:       postgres.NewInstanceAuthProviderRepository(db),
		Policies:        postgres.NewInstanceAuthPolicyRepository(db),
		Credentials:     postgres.NewUserCredentialRepository(db),
		RootAdminEmails: admins.RootAdminEmails,
	})
}

// newRootAdminPasswordService builds the password recovery over the database,
// with the server's hashing parameters. It has no event publisher, so it sets
// passwords on existing accounts only. The password service is built for
// SetPassword alone: it gets no policy or allowlist resolver, which only
// sign-in reads.
func newRootAdminPasswordService(db *database.DB, cfg *config.Config) *services.RootAdminPasswordService {
	// The CLI's own output is the report; the services' log lines stay quiet.
	logger := slog.New(slog.DiscardHandler)
	users := postgres.NewUserRepository(db)
	credentials := postgres.NewUserCredentialRepository(db)
	admins := services.NewInstanceAdminService(cfg.Auth.InstanceAdmins, nil, nil, logger)
	return services.NewRootAdminPasswordService(services.RootAdminPasswordDeps{
		Users:       users,
		Credentials: credentials,
		Policies:    postgres.NewInstanceAuthPolicyRepository(db),
		Passwords: services.NewPasswordAuthService(services.PasswordAuthDeps{
			Users: users, Credentials: credentials, Hasher: password.NewHasher(password.DefaultParams), Logger: logger,
		}),
		IsRootAdmin: admins.IsRootAdmin,
		Logger:      logger,
	})
}

// authSettingsTables are the tables every `vibexp admin auth` command may touch.
var authSettingsTables = []string{
	"instance_auth_providers",
	"instance_auth_allowlist",
	"instance_auth_setup",
	"instance_auth_settings_version",
	"instance_settings_audit",
	"instance_auth_policy",
	"users",
	"user_credentials",
}

// requireAuthSettingsTables fails, naming the first missing table, unless the
// schema holds every auth settings table.
func requireAuthSettingsTables(ctx context.Context, db *database.DB) error {
	for _, table := range authSettingsTables {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
			return fmt.Errorf("failed to check the database schema: %w", err)
		}
		if !exists {
			return fmt.Errorf("the database has no %s table, so it is not migrated to this version; "+
				"start the server once to run the migrations, then retry (this command never migrates)", table)
		}
	}
	return nil
}
