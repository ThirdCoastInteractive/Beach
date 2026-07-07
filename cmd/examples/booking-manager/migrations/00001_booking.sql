-- booking-manager domain schema: rental properties with key codes, guest
-- inquiries and bookings, a small staffing suite (hiring pipeline, shifts,
-- time clock), and per-property supply inventory.
--
-- The identity/session/RBAC tables (users, user_credentials_local, roles,
-- role_permissions, user_roles, sessions, password_reset_tokens) are supplied
-- by the framework's own embedded migration sets (auth.Migrations +
-- session.Migrations), which boot wiring runs ahead of this file. This
-- migration owns only the booking tables and seeds the two roles the app uses.

-- +goose Up

-- Rental properties: the cottages/cabins an operator manages. lock_device_id
-- names the smart-lock device the locks provider programs door codes onto
-- (empty = no connected lock).
-- +goose StatementBegin
CREATE TABLE bm_properties (
    id                 bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name               text        NOT NULL,
    description        text        NOT NULL DEFAULT '',
    address            text        NOT NULL DEFAULT '',
    sleeps             int         NOT NULL DEFAULT 2,
    bedrooms           int         NOT NULL DEFAULT 1,
    nightly_rate_cents bigint      NOT NULL DEFAULT 0,
    lock_device_id     text        NOT NULL DEFAULT '',
    notes              text        NOT NULL DEFAULT '',
    created_at         timestamptz NOT NULL DEFAULT now(),
    deleted_at         timestamptz
);
-- +goose StatementEnd

-- Standing key codes per property (lockbox, gate, shed, ...). Distinct from a
-- booking's one-off door code, which lives on the booking row.
-- +goose StatementBegin
CREATE TABLE bm_key_codes (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    property_id bigint      NOT NULL REFERENCES bm_properties (id) ON DELETE CASCADE,
    label       text        NOT NULL,
    code        text        NOT NULL,
    active      boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX bm_key_codes_property_idx ON bm_key_codes (property_id);
-- +goose StatementEnd

-- Guest intake: an inquiry from the public form works a small pipeline
-- (new -> quoted -> won|lost); "won" converts into a booking.
-- +goose StatementBegin
CREATE TABLE bm_inquiries (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    property_id bigint      REFERENCES bm_properties (id) ON DELETE SET NULL,
    name        text        NOT NULL,
    email       text        NOT NULL,
    phone       text        NOT NULL DEFAULT '',
    party_size  int         NOT NULL DEFAULT 2,
    check_in    date,
    check_out   date,
    message     text        NOT NULL DEFAULT '',
    status      text        NOT NULL DEFAULT 'new', -- new|quoted|won|lost
    created_at  timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- Bookings: a dated stay at one property. door_code is the per-stay code the
-- locks provider programs on confirmation and the guest receives by mail/SMS.
-- +goose StatementBegin
CREATE TABLE bm_bookings (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    property_id bigint      NOT NULL REFERENCES bm_properties (id) ON DELETE CASCADE,
    inquiry_id  bigint      REFERENCES bm_inquiries (id) ON DELETE SET NULL,
    guest_name  text        NOT NULL,
    guest_email text        NOT NULL DEFAULT '',
    guest_phone text        NOT NULL DEFAULT '',
    check_in    date        NOT NULL,
    check_out   date        NOT NULL,
    status      text        NOT NULL DEFAULT 'pending', -- pending|confirmed|checked_in|checked_out|cancelled
    rate_cents  bigint      NOT NULL DEFAULT 0,
    door_code   text        NOT NULL DEFAULT '',
    notes       text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX bm_bookings_property_dates_idx ON bm_bookings (property_id, check_in, check_out);
-- +goose StatementEnd

-- Hiring pipeline: applicants advance applied -> interview -> offer ->
-- hired|rejected; "hired" creates a bm_staff row.
-- +goose StatementBegin
CREATE TABLE bm_applicants (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       text        NOT NULL,
    email      text        NOT NULL,
    phone      text        NOT NULL DEFAULT '',
    role       text        NOT NULL DEFAULT 'cleaner', -- cleaner|maintenance|manager
    stage      text        NOT NULL DEFAULT 'applied', -- applied|interview|offer|hired|rejected
    notes      text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE bm_staff (
    id                bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name              text        NOT NULL,
    email             text        NOT NULL DEFAULT '',
    phone             text        NOT NULL DEFAULT '',
    role              text        NOT NULL DEFAULT 'cleaner',
    hourly_rate_cents bigint      NOT NULL DEFAULT 0,
    active            boolean     NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- Scheduled work: a shift assigns one staffer to a property (or to general
-- work when property_id is null) for a time window.
-- +goose StatementBegin
CREATE TABLE bm_shifts (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    staff_id    bigint      NOT NULL REFERENCES bm_staff (id) ON DELETE CASCADE,
    property_id bigint      REFERENCES bm_properties (id) ON DELETE SET NULL,
    starts_at   timestamptz NOT NULL,
    ends_at     timestamptz NOT NULL,
    kind        text        NOT NULL DEFAULT 'cleaning', -- cleaning|turnover|maintenance|greeting
    notes       text        NOT NULL DEFAULT ''
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX bm_shifts_time_idx ON bm_shifts (starts_at);
-- +goose StatementEnd

-- Time clock: an open entry (clock_out null) is an on-the-clock staffer.
-- +goose StatementBegin
CREATE TABLE bm_time_entries (
    id        bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    staff_id  bigint      NOT NULL REFERENCES bm_staff (id) ON DELETE CASCADE,
    clock_in  timestamptz NOT NULL DEFAULT now(),
    clock_out timestamptz
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX bm_time_entries_staff_idx ON bm_time_entries (staff_id, clock_in);
-- +goose StatementEnd

-- Supply inventory per property: kitchen stock, cleaning supplies, linens.
-- par is the restock level; qty at or under par shows as low stock.
-- +goose StatementBegin
CREATE TABLE bm_supplies (
    id          bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    property_id bigint  NOT NULL REFERENCES bm_properties (id) ON DELETE CASCADE,
    name        text    NOT NULL,
    category    text    NOT NULL DEFAULT 'kitchen', -- kitchen|cleaning|linens|other
    qty         numeric NOT NULL DEFAULT 0,
    par         numeric NOT NULL DEFAULT 0,
    unit        text    NOT NULL DEFAULT 'ea'
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX bm_supplies_property_idx ON bm_supplies (property_id);
-- +goose StatementEnd

-- Seed the two roles the app gates on. Permissions are flat "resource:action"
-- strings the auth layer flattens onto the Principal at login. owner runs the
-- whole operation; staff can see bookings and the schedule and keep inventory
-- counts honest, but can't write bookings or manage hiring.
-- +goose StatementBegin
INSERT INTO roles (slug) VALUES ('owner'), ('staff');
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO role_permissions (role_id, permission)
SELECT id, perm
  FROM roles
  CROSS JOIN (VALUES
    ('bookings:read'),
    ('bookings:write'),
    ('properties:write'),
    ('staffing:read'),
    ('staffing:write'),
    ('inventory:write')
  ) AS p(perm)
 WHERE roles.slug = 'owner';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO role_permissions (role_id, permission)
SELECT id, perm
  FROM roles
  CROSS JOIN (VALUES
    ('bookings:read'),
    ('staffing:read'),
    ('inventory:write')
  ) AS p(perm)
 WHERE roles.slug = 'staff';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS bm_supplies;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS bm_time_entries;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS bm_shifts;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS bm_staff;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS bm_applicants;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS bm_bookings;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS bm_inquiries;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS bm_key_codes;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS bm_properties;
-- +goose StatementEnd
