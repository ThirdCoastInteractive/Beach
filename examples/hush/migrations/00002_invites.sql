-- +goose Up
-- Invites: one row per minted code. The chain edge (inviter → used_by) feeds
-- Repute's chain track — an invitee's 👍/👎 echo onto their inviter — and
-- counting a member's minted rows enforces the allowance throttle.
CREATE TABLE hush_invites (
    code       text        PRIMARY KEY,
    inviter    text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    used_by    text,
    used_at    timestamptz
);

CREATE INDEX hush_invites_inviter_idx ON hush_invites (inviter);

-- Signups ledger: one row per account created outside the admin queue, so the
-- open-registration cap (HUSH_OPEN_SLOTS) can count what it has spent.
CREATE TABLE hush_signups (
    id         bigserial   PRIMARY KEY,
    handle     text        NOT NULL,
    via        text        NOT NULL, -- open | invite
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE hush_signups;
DROP TABLE hush_invites;
