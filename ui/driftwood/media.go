package driftwood

import (
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/a-h/templ"
)

// --- Media -------------------------------------------------------------------

func (k Kit) Avatar(p ui.AvatarProps) templ.Component {
	cls := "dw-avatar" + sizeClass("dw-avatar", p.Size)
	var inner templ.Component
	if p.Src != "" {
		a := ` src="` + esc(p.Src) + `" alt="` + esc(p.Alt) + `" loading="lazy" decoding="async"`
		inner = raw(`<img class="dw-avatar-img"` + a + `>`)
	} else {
		inner = el("span", "dw-avatar-initials", ` aria-hidden="true"`, txt(p.Initials))
	}
	body := []templ.Component{inner}
	if p.Status != "" {
		body = append(body, el("span", "dw-avatar-status dw-status-"+esc(p.Status),
			` role="img" aria-label="`+esc(p.Status)+`"`))
	}
	return el("span", cls, attrs(p.Attrs), body...)
}

func (k Kit) Image(p ui.ImageProps) templ.Component {
	// Every image sits in an aspect box so late bytes fill reserved pixels
	// (the no-pop-in law applied to media).
	loading := "lazy"
	a := ` src="` + esc(p.Src) + `" alt="` + esc(p.Alt) + `"`
	if p.Set != "" {
		a += ` srcset="` + esc(p.Set) + `"`
	}
	if p.Sizes != "" {
		a += ` sizes="` + esc(p.Sizes) + `"`
	}
	if p.Eager {
		loading = "eager"
		a += ` fetchpriority="high"`
	}
	a += ` loading="` + loading + `" decoding="async"`
	fit := p.Fit
	if fit == "" {
		fit = ui.FitCover
	}
	img := raw(`<img class="dw-img dw-fit-` + string(fit) + `"` + a + `>`)
	return k.AspectBox(ui.AspectBoxProps{Ratio: p.Ratio, Attrs: p.Attrs}, img)
}

// Figure is an image with a caption (<figure>/<figcaption>). The embedded image
// keeps the media perf laws (aspect box, lazy/eager, async decode).
func (k Kit) Figure(p ui.FigureProps) templ.Component {
	body := make([]templ.Component, 0, 2)
	body = append(body, k.Image(p.Image))
	if p.Caption != "" {
		body = append(body, el("figcaption", "dw-figcaption", "", txt(p.Caption)))
	}
	return el("figure", "dw-figure", attrs(p.Attrs), body...)
}

func (Kit) AspectBox(p ui.AspectBoxProps, inner ...templ.Component) templ.Component {
	cls := "dw-aspect"
	if p.Ratio != "" {
		cls += " dw-aspect-" + string(p.Ratio)
	}
	return el("div", cls, attrs(p.Attrs), inner...)
}
