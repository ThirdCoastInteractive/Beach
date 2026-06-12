-- +goose Up
-- Applications carry an optional contact email: where the approve/reject
-- notice goes, and the address copied into hush_member_emails (unverified)
-- when the applicant becomes a member. Admin eyes only until then.
ALTER TABLE hush_applications ADD COLUMN email text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE hush_applications DROP COLUMN email;
