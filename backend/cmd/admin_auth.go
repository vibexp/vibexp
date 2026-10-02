package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

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

	auth.AddCommand(providers, allowlist, setup)
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
			_, err := fmt.Fprintf(out, "Identity provider %q is already %s; no change.\n", slug, state)
			return err
		}
		if _, err := fmt.Fprintf(out, "Identity provider %q %s.\n", slug, state); err != nil {
			return err
		}
		if result.NoneEnabled {
			_, err := fmt.Fprintln(out, "No identity provider is enabled now, so the instance is in setup mode. "+
				"Run `vibexp admin auth setup rearm` for a setup URL.")
			return err
		}
		return nil
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
		Providers: postgres.NewInstanceAuthProviderRepository(db),
		Allowlist: postgres.NewInstanceAuthAllowlistRepository(db),
		Setup:     postgres.NewInstanceAuthSetupRepository(db),
		Version:   postgres.NewInstanceAuthSettingsVersionRepository(db),
		BaseURL:   cfg.Frontend.BaseURL,
	})
}

// authSettingsTables are the tables every `vibexp admin auth` command may touch.
var authSettingsTables = []string{
	"instance_auth_providers",
	"instance_auth_allowlist",
	"instance_auth_setup",
	"instance_auth_settings_version",
	"instance_settings_audit",
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
