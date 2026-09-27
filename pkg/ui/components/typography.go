package components

import "github.com/ThirdCoastInteractive/Beach/pkg/datastar"

type TextRole string

const (
	TextBody    TextRole = ""
	TextCaption TextRole = "caption"
	TextLabel   TextRole = "label"
	TextMono    TextRole = "mono"
	TextDisplay TextRole = "display"
)

type TextProps struct {
	Value string
	Role  TextRole
	Attrs datastar.Attrs
}

func textRoleClass(r TextRole) string {
	switch r {
	case TextCaption:
		return "dw-text-caption"
	case TextLabel:
		return "dw-text-label"
	case TextMono:
		return "dw-text-mono"
	case TextDisplay:
		return "dw-text-display"
	}
	return "dw-text-body"
}
