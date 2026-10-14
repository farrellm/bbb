// Command server runs BBB Watch: the poller and the HTTP API (plus the
// built frontend when WEB_DIR is set) in one process. A few flags instead
// run a one-off admin task and exit.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/farrellm/bbb/internal/auth"
	"github.com/farrellm/bbb/internal/bbb"
	"github.com/farrellm/bbb/internal/config"
	"github.com/farrellm/bbb/internal/db"
	"github.com/farrellm/bbb/internal/handler"
	"github.com/farrellm/bbb/internal/notify"
	"github.com/farrellm/bbb/internal/scraper"
	"github.com/farrellm/bbb/internal/store"
)

var (
	once   = flag.Bool("scrape-once", false, "poll the page once, print the outcome, then exit")
	noPoll = flag.Bool("no-poll", false, "serve the API without polling the page")

	createInvite = flag.Bool("create-invite", false, "create an invite link and exit")
	inviteUses   = flag.Int("max-uses", handler.DefaultInviteUses, "with -create-invite: how many accounts it can create")
	inviteDays   = flag.Int("expires-days", handler.DefaultInviteDays, "with -create-invite: days until it expires, 0 for never")
	listUsers    = flag.Bool("list-users", false, "list accounts and exit")
	deleteUser   = flag.String("delete-user", "", "delete the account with this email and exit")
	makeAdmin    = flag.String("make-admin", "", "make the account with this email an admin and exit")
	genVAPID     = flag.Bool("gen-vapid", false, "print a new VAPID key pair for VAPID_PUBLIC and VAPID_PRIVATE, then exit")
)

func main() {
	flag.Parse()
	if err := run(context.Background()); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *genVAPID {
		pub, priv, err := notify.GenerateKeys()
		if err != nil {
			return err
		}
		fmt.Printf("export VAPID_PUBLIC=%s\nexport VAPID_PRIVATE=%s\n", pub, priv)
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database connect: %w", err)
	}
	defer pool.Close()
	st := store.New(pool)

	if done, err := adminCommand(ctx, cfg, st); done {
		return err
	}

	log := slog.Default()
	var sender *notify.Sender
	if cfg.PushConfigured() {
		sender = &notify.Sender{PublicKey: cfg.VAPIDPublic, PrivateKey: cfg.VAPIDPrivate, Subject: cfg.VAPIDSubject}
	}
	notifier := notify.New(st, sender, log)

	sc := &scraper.Scraper{
		Fetcher:       bbb.NewFetcher(cfg.BBBURL, cfg.UserAgent),
		Store:         st,
		Signup:        bbb.SignupDetector{Hosts: cfg.SignupHosts},
		Location:      cfg.SchoolTZ,
		Interval:      cfg.PollInterval,
		NightInterval: cfg.NightPollInterval,
		Log:           log,
		OnEvents:      notifier.Kick,
	}

	if *once {
		out, err := sc.RunOnce(ctx)
		fmt.Printf("run %d: unchanged=%v baseline=%v suspect=%v items=%d events=%d\n",
			out.RunID, out.Unchanged, out.Baseline, out.Suspect, out.Items, out.Events)
		if errors.Is(err, scraper.ErrSuspect) {
			return nil
		}
		return err
	}

	api := &handler.API{
		Store:       st,
		Sessions:    auth.NewSessions(cfg.SessionSecret, cfg.SecureCookies()),
		Flows:       auth.NewFlows(cfg.SessionSecret, cfg.SecureCookies()),
		PublicURL:   cfg.PublicURL,
		AdminEmails: cfg.AdminEmails,
		PageURL:     cfg.BBBURL,
		WebDir:      cfg.WebDir,
		Notifier:    notifier,
	}
	if cfg.GoogleConfigured() {
		api.Google = auth.NewGoogle(ctx, auth.GoogleProvider, cfg.GoogleClientID, cfg.GoogleClientSecret,
			cfg.PublicURL+"/api/auth/google/callback")
	} else {
		log.Warn("GOOGLE_CLIENT_ID or GOOGLE_CLIENT_SECRET not set; sign-in is disabled")
	}
	if !notifier.Enabled() {
		log.Warn("VAPID_PUBLIC or VAPID_PRIVATE not set; push notifications are off")
	}
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		log.Info("http listening", "addr", cfg.ListenAddr, "public_url", cfg.PublicURL)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	g.Go(func() error {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown) //nolint:contextcheck // shutdown must outlive the cancelled ctx
	})
	g.Go(func() error {
		notifier.Run(ctx)
		return nil
	})
	if !*noPoll {
		g.Go(func() error {
			log.Info("poller started", "url", cfg.BBBURL, "interval", cfg.PollInterval)
			sc.Run(ctx)
			return nil
		})
	}
	return g.Wait()
}

// adminCommand runs the flag-selected admin task, if any, and reports
// whether it did.
func adminCommand(ctx context.Context, cfg config.Config, st *store.Store) (bool, error) {
	switch {
	case *createInvite:
		var expires *time.Time
		if *inviteDays > 0 {
			t := time.Now().Add(time.Duration(*inviteDays) * 24 * time.Hour)
			expires = &t
		}
		inv, err := st.CreateInvite(ctx, auth.RandomToken(), 0, *inviteUses, expires)
		if err != nil {
			return true, err
		}
		fmt.Println(handler.InviteURL(cfg.PublicURL, inv.Code))
		return true, nil
	case *listUsers:
		users, err := st.ListUsers(ctx)
		if err != nil {
			return true, err
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tEMAIL\tNAME\tADMIN\tLAST SIGN-IN")
		for _, u := range users {
			last := "never"
			if u.LastLoginAt != nil {
				last = u.LastLoginAt.Local().Format(time.DateTime)
			}
			_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%v\t%s\n", u.ID, u.Email, u.Name, u.IsAdmin, last)
		}
		return true, tw.Flush()
	case *deleteUser != "":
		return true, st.DeleteUser(ctx, *deleteUser)
	case *makeAdmin != "":
		return true, st.MakeAdmin(ctx, *makeAdmin)
	}
	return false, nil
}
