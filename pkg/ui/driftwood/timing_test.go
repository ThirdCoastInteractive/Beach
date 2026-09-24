package driftwood

// The criteria this pass closed, one test each. They live apart from a11y_test.go
// because those are laws applied to every component, and these are claims about
// specific ones.

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"

	"github.com/ThirdCoastInteractive/Beach/pkg/prefs"
)

// --- timing (WCAG 2.2.1 / 2.2.2) -------------------------------------------------

// TestToastNeverExpiresWhenItCarriesWork is the guard that makes an
// on-by-default timer defensible.
//
// A toast that fades is a time limit, and SC 2.2.1 constrains those. The default
// is on because the docs have always promised auto-dismiss, and because a stream
// of confirmations that never leave is its own problem — but a message someone
// has to act on must not be the one that vanishes while they are deciding.
func TestToastNeverExpiresWhenItCarriesWork(t *testing.T) {
	persists := map[string]ToastProps{
		"danger carries an error to fix":  {Role: RoleDanger, Title: "Save failed"},
		"warn carries something to weigh": {Role: RoleWarn, Title: "Slow link"},
		"an explicit ToastPersist":        {Role: RoleGood, Title: "Saved", Dismiss: ToastPersist},
	}
	for why, p := range persists {
		if out := render(t, Toast(p)); strings.Contains(out, "data-dismiss") {
			t.Errorf("%s, and yet the toast expires on a timer\n%s", why, out)
		}
	}

	// The ordinary case does expire, or the feature does not exist.
	if out := render(t, Toast(ToastProps{Role: RoleGood, Title: "Saved"})); !strings.Contains(out, "data-dismiss") {
		t.Errorf("a routine toast never expires, so the stack grows without limit\n%s", out)
	}
}

// TestVisitorCanTurnOffToastTimers covers SC 2.2.1's first and strongest remedy:
// the limit can be turned off before it is ever encountered. Without it the other
// guards are mitigation rather than conformance.
func TestVisitorCanTurnOffToastTimers(t *testing.T) {
	ctx := prefs.With(context.Background(), prefs.Prefs{LiveUpdates: true, AutoDismiss: false})
	var b bytes.Buffer
	toast := Toast(ToastProps{Role: RoleGood, Title: "Saved", Dismiss: 3 * time.Second})
	if err := toast.Render(ctx, &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(b.String(), "data-dismiss") {
		t.Errorf("the visitor turned auto-dismiss off and a toast still expires\n%s", b.String())
	}
}

// TestLiveToggleStopsOpeningTheStream is the mechanism behind SC 2.2.2.
//
// Pausing here is not hiding updates that keep arriving: the element that opens
// the connection is simply not rendered, so nothing on a paused page holds a
// stream at all. If this fails, the button is lying.
func TestLiveToggleStopsOpeningTheStream(t *testing.T) {
	p := LiveToggleProps{ID: "live", Stream: "/live", Label: "Board"}

	running := render(t, LiveToggle(p))
	if !strings.Contains(running, "data-init") || !strings.Contains(running, "/live") {
		t.Errorf("a live page does not open its stream\n%s", running)
	}
	if !strings.Contains(running, `aria-pressed="false"`) {
		t.Errorf("the control does not report that updates are running\n%s", running)
	}

	ctx := prefs.With(context.Background(), prefs.Prefs{LiveUpdates: false, AutoDismiss: true})
	var b bytes.Buffer
	if err := LiveToggle(p).Render(ctx, &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	paused := b.String()
	if strings.Contains(paused, "data-init") {
		t.Errorf("a paused page still opens a stream, so the pause is cosmetic\n%s", paused)
	}
	if !strings.Contains(paused, `id="live"`) {
		t.Errorf("the patch target vanished with the stream, so a fragment aimed at it lands nowhere\n%s", paused)
	}
	if !strings.Contains(paused, `aria-pressed="true"`) {
		t.Errorf("the control does not report that updates are paused\n%s", paused)
	}
}

// TestOneToggleStopsEveryStream covers the multi-stream page. boardwalk opens
// three, and a control that stopped one of them would be worse than none: the
// board would freeze while the hand and the race kept moving, which reads as a
// broken page rather than a paused one.
func TestOneToggleStopsEveryStream(t *testing.T) {
	paused := prefs.With(context.Background(), prefs.Prefs{LiveUpdates: false, AutoDismiss: true})
	for _, c := range []struct {
		name string
		c    templ.Component
	}{
		{"the toggle's own stream", LiveToggle(LiveToggleProps{ID: "board", Stream: "/board"})},
		{"a sibling stream", LiveStream(LiveStreamProps{ID: "hand", Stream: "/hand"})},
	} {
		var live, off bytes.Buffer
		if err := c.c.Render(context.Background(), &live); err != nil {
			t.Fatalf("render: %v", err)
		}
		if err := c.c.Render(paused, &off); err != nil {
			t.Fatalf("render: %v", err)
		}
		if !strings.Contains(live.String(), "data-init") {
			t.Errorf("%s never opens\n%s", c.name, live.String())
		}
		if strings.Contains(off.String(), "data-init") {
			t.Errorf("%s keeps running after the pause\n%s", c.name, off.String())
		}
	}
}

// --- media (WCAG 1.2.x, 1.4.2, 2.2.2) --------------------------------------------

// TestVideoNeverAutoplaysWithoutControls pins the rule that decides whether a
// background video is conformant.
//
// Motion that starts on its own and runs past five seconds beside other content
// needs a way to stop it (SC 2.2.2). The kit's answer is the native control bar,
// because the alternative — a scripted pause button — is local interactivity in
// script, which this kit does not do. So autoplay without controls is a shape the
// component cannot be made to produce.
func TestVideoNeverAutoplaysWithoutControls(t *testing.T) {
	out := render(t, Video(VideoProps{Src: "/v.mp4", Background: true, Ratio: RatioWide}))
	if !strings.Contains(out, "autoplay") || !strings.Contains(out, "loop") {
		t.Fatalf("Background did not produce a looping autoplay video\n%s", out)
	}
	if !strings.Contains(out, "controls") {
		t.Errorf("autoplaying motion with no way to stop it (WCAG 2.2.2)\n%s", out)
	}
	if !strings.Contains(out, "muted") {
		t.Errorf("audio that starts on its own (WCAG 1.4.2)\n%s", out)
	}
}

// TestVideoReservesItsFrameAndWaits is the two perf laws applied to media: the
// box is reserved before any bytes arrive, and no bytes arrive until asked for.
func TestVideoReservesItsFrameAndWaits(t *testing.T) {
	out := render(t, Video(VideoProps{Src: "/v.mp4", Poster: "/v.jpg", Ratio: RatioPhoto, Label: "Tour"}))
	if !strings.Contains(out, "dw-aspect-photo") {
		t.Errorf("no reserved frame, so late media shoves the page down\n%s", out)
	}
	if !strings.Contains(out, `preload="none"`) {
		t.Errorf("video bytes fetched before anyone asked for them\n%s", out)
	}
	if !strings.Contains(out, `poster="/v.jpg"`) {
		t.Errorf("nothing to look at before play\n%s", out)
	}
}

// TestVideoTrackDefaultsToCaptions checks the default that matters. Captions
// carry the non-speech sound that carries meaning; subtitles do not. A caller who
// has not thought about the difference is better served by the more complete of
// the two.
func TestVideoTrackDefaultsToCaptions(t *testing.T) {
	out := render(t, Video(VideoProps{
		Src:    "/v.mp4",
		Ratio:  RatioWide,
		Tracks: []Track{{Src: "/v.vtt", Lang: "en", Label: "English"}},
	}))
	if !strings.Contains(out, `kind="captions"`) {
		t.Errorf("a track with no kind did not default to captions\n%s", out)
	}
}

// TestImageSourcesRenderAPicture covers the documented format fallback that was
// never built: srcset switches resolution for one crop, <picture> is what carries
// a different format or a different crop entirely.
func TestImageSourcesRenderAPicture(t *testing.T) {
	out := render(t, Image(ImageProps{
		Src: "/a.jpg", Alt: "Oats", Ratio: RatioPhoto,
		Sources: []Source{
			{Src: "/a.avif", Type: "image/avif"},
			{Src: "/a-wide.webp", Type: "image/webp", Media: "(min-width: 40rem)"},
		},
	}))
	for _, want := range []string{"<picture>", `type="image/avif"`, `media="(min-width: 40rem)"`, `alt="Oats"`} {
		if !strings.Contains(out, want) {
			t.Errorf("picture missing %s\n%s", want, out)
		}
	}
	// The <img> stays last so a browser understanding none of the sources still
	// gets a picture, and the alt still comes from one place.
	if strings.Index(out, "<img") < strings.LastIndex(out, "<source") {
		t.Errorf("the fallback <img> is not last inside <picture>\n%s", out)
	}
}

// --- confirm (WCAG 3.3.4) --------------------------------------------------------

// TestConfirmDescribesItsConsequence is the whole point of the component.
//
// The extra click is not what SC 3.3.4 buys; the sentence is. A dialog that asks
// "are you sure?" without saying what happens is a dialog people click through.
// Wiring the message as the dialog's description means it is announced *with* the
// name rather than only if someone goes looking.
func TestConfirmDescribesItsConsequence(t *testing.T) {
	out := render(t, Confirm(ConfirmProps{
		ID: "del", Title: "Delete booking?",
		Message: "The guest is notified and this cannot be undone.",
		Post:    "/x",
	}))
	// The description has to be on the <dialog>. This assertion is deliberately
	// positional rather than "does the string appear anywhere", because the first
	// cut of this component put aria-describedby on a plain <div> inside the
	// dialog — which renders, passes a looser test, and is announced by nothing,
	// since a div is not a widget with a description to read.
	head := out[strings.Index(out, "<dialog") : strings.Index(out, ">")+1]
	if !strings.Contains(head, `aria-describedby="del-message"`) {
		t.Errorf("the consequence is not the *dialog's* description\n%s", head)
	}
	if !strings.Contains(out, `id="del-message"`) {
		t.Errorf("the description points at nothing\n%s", out)
	}

	// Nothing to describe means no dangling reference: an aria-describedby naming
	// an element that does not exist is announced as nothing and caught by no
	// validator.
	bare := render(t, Confirm(ConfirmProps{ID: "del", Title: "Delete?", Post: "/x"}))
	if strings.Contains(bare, "aria-describedby") {
		t.Errorf("a Confirm with no message still claims a description\n%s", bare)
	}
	// Cancel comes first *and* carries autofocus. Order alone is not enough:
	// Modal renders a close X in its header, so the first focusable thing in a
	// Confirm is that X, and a native <dialog> focuses whatever comes first. The
	// safe answer should be the one already under the finger.
	cancel := strings.Index(out, "Cancel")
	confirm := strings.Index(out, "Confirm")
	if cancel < 0 || confirm < 0 || cancel > confirm {
		t.Errorf("the destructive button comes before Cancel\n%s", out)
	}
	if !strings.Contains(out[:cancel], "autofocus") {
		t.Errorf("opening the dialog does not land on Cancel\n%s", out)
	}
}

// --- language (WCAG 3.1.2) -------------------------------------------------------

// TestLangMarksAPassage checks that a run of another language carries both its
// tag and its direction: a screen reader switches voice on the first, and the
// text lays out correctly on the second.
func TestLangMarksAPassage(t *testing.T) {
	fr := render(t, Lang("fr"), text("liberte"))
	if !strings.Contains(fr, `lang="fr"`) || !strings.Contains(fr, `dir="ltr"`) {
		t.Errorf("a French run was not marked\n%s", fr)
	}
	ar := render(t, LangBlock("ar-EG"), text("marhaban"))
	if !strings.Contains(ar, `lang="ar-EG"`) || !strings.Contains(ar, `dir="rtl"`) {
		t.Errorf("an Arabic passage did not get its direction\n%s", ar)
	}
}
