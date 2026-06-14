// The text half of each outbound message, paired with its HTML component from
// emails.templ and returned as one *mailer.Email. Text is the canonical body
// (mailer.Email's contract — every transport sends it); the HTML is the same
// words on the white shell. Callers build absolute links from the web config's
// BaseURL and queue the result with Outbox.EnqueueMail (outbox.go).
package mail

import (
	"context"
	"strings"

	"github.com/ThirdCoastInteractive/the-beach/pkg/mailer"
	"github.com/a-h/templ"
)

// renderEmail renders an emails.templ component to the HTML string an outbox
// row stores. Render only fails on writer errors; strings.Builder has none.
func renderEmail(c templ.Component) string {
	var b strings.Builder
	_ = c.Render(context.Background(), &b)
	return b.String()
}

// VerifyEmail carries the tokened verification link for a member's address.
func VerifyEmail(link string) *mailer.Email {
	return &mailer.Email{
		Subject: "verify your email",
		Text: "hush\n\nverify your email — your standing will feel it.\n\n" + link +
			"\n\nyour address is never shown to anyone. if you didn't add it to a hush account, ignore this.\n",
		HTML: renderEmail(verifyEmailHTML(link)),
	}
}

// ApplicationReceivedAdmin is the admin's new-application notice.
func ApplicationReceivedAdmin(handle, pitch string) *mailer.Email {
	return &mailer.Email{
		Subject: "new application: " + handle,
		Text: "hush\n\nsomeone is at the door.\n\nhandle: " + handle + "\n\n" + pitch +
			"\n\nthe queue is waiting.\n",
		HTML: renderEmail(applicationReceivedAdminHTML(handle, pitch)),
	}
}

// ApplicationApproved tells an applicant they're in. verifyLink may be empty —
// it rides along only while the address is unverified.
func ApplicationApproved(handle, loginURL, verifyLink string) *mailer.Email {
	text := "hush\n\n" + handle + " — your application was approved. post into the quiet.\n\nsign in: " + loginURL + "\n"
	if verifyLink != "" {
		text += "\nverify this address while you're here — your standing will feel it:\n\n" + verifyLink + "\n"
	}
	return &mailer.Email{
		Subject: "you're in",
		Text:    text,
		HTML:    renderEmail(applicationApprovedHTML(handle, loginURL, verifyLink)),
	}
}

// ApplicationRejected is a quiet no.
func ApplicationRejected(handle string) *mailer.Email {
	return &mailer.Email{
		Subject: "about your application",
		Text: "hush\n\n" + handle + " — your application wasn't approved this time.\n\n" +
			"the wall stays small on purpose. you may apply again.\n",
		HTML: renderEmail(applicationRejectedHTML(handle)),
	}
}

// InviteEmail mails a minted code as its redemption link. One use.
func InviteEmail(link string) *mailer.Email {
	return &mailer.Email{
		Subject: "you're invited",
		Text: "hush\n\nsomeone minted you a way in. the link works once:\n\n" + link +
			"\n\nwhoever invited you answers for you — post things people like.\n",
		HTML: renderEmail(inviteEmailHTML(link)),
	}
}
