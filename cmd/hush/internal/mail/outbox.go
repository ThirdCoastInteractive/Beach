// Package mail is hush's transactional email: handlers enqueue rows into
// hush_outbox and a background pump delivers them through the Mailer with retry
// + backoff. The queue is the seam — a handler never blocks on SMTP, a crashed
// send is just a row still pending, and the dev default (mailer.LogMailer)
// "delivers" to the terminal so the flows work on a fresh checkout. The email
// builders (emails.go + emails.templ) live alongside the pump.
package mail

import (
	"context"
	"log/slog"
	"time"

	"github.com/ThirdCoastInteractive/the-beach/pkg/mailer"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The pump's knobs: claim cadence, claim size, and how many tries a message
// gets before its row is parked as failed for an admin to read.
const (
	outboxEvery    = 5 * time.Second
	outboxBatch    = 10
	outboxMaxTries = 8
)

// Outbox is the Postgres-backed mail queue and its sender loop.
type Outbox struct {
	pool   *pgxpool.Pool
	mailer mailer.Mailer
}

// New wraps the pool and mailer.
func New(p *pgxpool.Pool, m mailer.Mailer) *Outbox {
	return &Outbox{pool: p, mailer: m}
}

// Enqueue inserts one pending outbox row. Best-effort by design: every email
// hush sends is a courtesy, so a queue failure logs and the request proceeds —
// nobody's signup should die because the mail table hiccuped.
func (o *Outbox) Enqueue(ctx context.Context, to, subject, text, html, templateKey string) {
	if to == "" {
		return
	}
	if _, err := o.pool.Exec(ctx,
		`INSERT INTO hush_outbox (to_email, subject, text_body, html_body, template_key)
		 VALUES ($1, $2, $3, $4, $5)`, to, subject, text, html, templateKey); err != nil {
		slog.Error("hush: outbox enqueue failed", "template", templateKey, "err", err)
	}
}

// EnqueueMail queues a built template (emails.go) for to — the unpacking every
// call site would otherwise repeat.
func (o *Outbox) EnqueueMail(ctx context.Context, to, templateKey string, m *mailer.Email) {
	o.Enqueue(ctx, to, m.Subject, m.Text, m.HTML, templateKey)
}

// Pump is the process-lifetime sender loop, started at boot. Each tick drains
// one batch. No clean shutdown is needed — an undelivered row is still pending
// after a restart — but the context guard stops the goroutine if boot ever
// grows a lifecycle worth cancelling.
func (o *Outbox) Pump(ctx context.Context) {
	t := time.NewTicker(outboxEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			o.drain(ctx)
		}
	}
}

// outboxRow is one claimed message.
type outboxRow struct {
	id       int64
	to       string
	subject  string
	text     string
	html     string
	attempts int
}

// drain claims one batch of due rows and delivers them. FOR UPDATE SKIP LOCKED
// is the claim: the rows stay locked while their sends run, so a second instance
// pumping the same table skips past them instead of double-sending. A failed
// send backs off (1m doubling, capped at an hour) until outboxMaxTries parks the
// row as failed.
func (o *Outbox) drain(ctx context.Context) {
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	rows, err := tx.Query(ctx,
		`SELECT id, to_email, subject, text_body, html_body, attempts FROM hush_outbox
		 WHERE status = 'pending' AND next_attempt_at <= now()
		 ORDER BY next_attempt_at
		 FOR UPDATE SKIP LOCKED
		 LIMIT $1`, outboxBatch)
	if err != nil {
		return
	}
	var batch []outboxRow
	for rows.Next() {
		var m outboxRow
		if err := rows.Scan(&m.id, &m.to, &m.subject, &m.text, &m.html, &m.attempts); err != nil {
			rows.Close()
			return
		}
		batch = append(batch, m)
	}
	rows.Close()
	if rows.Err() != nil {
		return
	}

	for _, m := range batch {
		if err := o.mailer.Send(&mailer.Email{To: m.to, Subject: m.subject, Text: m.text, HTML: m.html}); err != nil {
			attempts := m.attempts + 1
			status := "pending"
			if attempts >= outboxMaxTries {
				status = "failed"
			}
			slog.Warn("hush: outbox send failed", "id", m.id, "attempts", attempts, "status", status, "err", err)
			_, _ = tx.Exec(ctx,
				`UPDATE hush_outbox SET attempts = $2, last_error = $3, status = $4,
				        next_attempt_at = now() + make_interval(secs => $5)
				 WHERE id = $1`,
				m.id, attempts, err.Error(), status, outboxBackoff(attempts).Seconds())
			continue
		}
		_, _ = tx.Exec(ctx, `UPDATE hush_outbox SET status = 'sent', sent_at = now() WHERE id = $1`, m.id)
	}
	_ = tx.Commit(ctx)
}

// outboxBackoff is the retry curve: 1m * 2^attempts, capped at an hour.
func outboxBackoff(attempts int) time.Duration {
	if attempts > 6 { // 1m << 6 already exceeds the cap
		attempts = 6
	}
	d := time.Minute << attempts
	if d > time.Hour {
		d = time.Hour
	}
	return d
}
