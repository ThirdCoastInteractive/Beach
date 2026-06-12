-- +goose Up
-- The wall moves to Postgres: posts, reactions, and the production foundations
-- the next phases wire — flags (community moderation), member emails (verified
-- contact + its reputation bump), and a mail outbox (transactional email with
-- retries). All tables land now so the schema is one migration, not five.

-- One row per post. body_html is already sanitized (renderPost) — never raw
-- user input. The ip/country/city/user_agent columns are the capture-time Meta,
-- admin eyes only; templates never render them. hidden_at is the moderation
-- switch: a hidden post stays (with its flags) for admin review, invisible to
-- the wall; hidden_by records who threw the switch ('flags' for the community).
CREATE TABLE hush_posts (
    id          bigserial   PRIMARY KEY,
    kind        text        NOT NULL, -- text | image
    body_html   text        NOT NULL DEFAULT '',
    media_url   text        NOT NULL DEFAULT '',
    cf_image_id text        NOT NULL DEFAULT '', -- Cloudflare Images id, once uploads move there
    author      text        NOT NULL DEFAULT '', -- handle; '' in database-less open mode
    ip          text        NOT NULL DEFAULT '',
    country     text        NOT NULL DEFAULT '',
    city        text        NOT NULL DEFAULT '',
    user_agent  text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    hidden_at   timestamptz,
    hidden_by   text        NOT NULL DEFAULT ''
);

-- The wall's one read shape: visible posts (hidden_at IS NULL), newest first.
CREATE INDEX hush_posts_wall_idx ON hush_posts (hidden_at, id DESC);

-- One reaction per voter per post, the first is final — the PK is the rule.
-- weight is +1/-1; rows replay into Repute at boot.
CREATE TABLE hush_reactions (
    post_id    bigint      NOT NULL REFERENCES hush_posts ON DELETE CASCADE,
    voter      text        NOT NULL,
    weight     smallint    NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, voter)
);

-- One flag per flagger per post. flagger_tier is captured at flag time: a flag
-- from FlagTier or above hides the post alone; FlagQuorum tier-0 flags agree
-- it away.
CREATE TABLE hush_flags (
    post_id      bigint      NOT NULL REFERENCES hush_posts ON DELETE CASCADE,
    flagger      text        NOT NULL,
    flagger_tier int         NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, flagger)
);

-- A member's (optional) email: set, verified by tokened link, worth
-- EmailPoints of author reputation once verified_at is stamped. The token is
-- stored hashed — the cleartext only ever lives in the email itself.
CREATE TABLE hush_member_emails (
    user_id           bigint      PRIMARY KEY REFERENCES users ON DELETE CASCADE,
    email             text        NOT NULL,
    verify_token_hash bytea,
    verified_at       timestamptz,
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- Transactional mail as data: handlers insert, a sender loop claims pending
-- rows by next_attempt_at and delivers with backoff. status is
-- pending | sent | failed.
CREATE TABLE hush_outbox (
    id              bigserial   PRIMARY KEY,
    to_email        text        NOT NULL,
    subject         text        NOT NULL,
    text_body       text        NOT NULL DEFAULT '',
    html_body       text        NOT NULL DEFAULT '',
    template_key    text        NOT NULL DEFAULT '',
    status          text        NOT NULL DEFAULT 'pending',
    attempts        int         NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error      text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    sent_at         timestamptz
);

-- The sender loop's read shape: pending rows whose retry time has come.
CREATE INDEX hush_outbox_pending_idx ON hush_outbox (status, next_attempt_at);

-- +goose Down
DROP TABLE hush_outbox;
DROP TABLE hush_member_emails;
DROP TABLE hush_flags;
DROP TABLE hush_reactions;
DROP TABLE hush_posts;
