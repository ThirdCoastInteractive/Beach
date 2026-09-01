# booking-manager

Beach example: small-operator rental manager. Postgres only. `pkg/mailer` and
`pkg/sms` fan out inquiry and confirmation messages (log transport when keys
are unset).

```
cp .env.example .env
# from Beach repo root:
make up-booking-manager
```

http://localhost:8080/ — seed `admin` / `password`. `/specimen` is the kit
gallery.

Mailgun/SMTP and Twilio env vars are optional.
