package hotkey

import "strings"

type Choice struct {
	Key   string
	Label string
}

func Choices() []Choice {
	return []Choice{
		{Key: "f8", Label: "F8"},
		{Key: "f9", Label: "F9"},
		{Key: "f10", Label: "F10"},
		{Key: "f11", Label: "F11"},
		{Key: "f12", Label: "F12"},
		{Key: "backtick", Label: "`"},
		{Key: "backslash", Label: `\`},
		{Key: "", Label: "Disabled"},
	}
}

func NormalizeKey(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "", "disabled", "none", "off":
		return ""
	case "`", "~", "tilde", "grave", "backtick":
		return "backtick"
	case `\`, "backslash", "pipe":
		return "backslash"
	case "f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12":
		return strings.ToLower(strings.TrimSpace(key))
	default:
		return strings.ToLower(strings.TrimSpace(key))
	}
}

func Label(key string) string {
	switch key := NormalizeKey(key); key {
	case "":
		return "Disabled"
	case "backtick":
		return "`"
	case "backslash":
		return `\`
	default:
		return strings.ToUpper(key)
	}
}
