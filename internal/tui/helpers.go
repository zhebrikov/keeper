package tui

import (
	"fmt"
	"strings"
)

func detailLine(key, value string) string {
	return detailKeyStyle.Render(key+":") + " " + detailValueStyle.Render(value)
}

func footerHelp(keys ...string) string {
	return helpStyle.Render(strings.Join(keys, "  •  "))
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func secretListTitle() string {
	return fmt.Sprintf("%s %s", titleStyle.Render("GophKeeper"), subtitleStyle.Render("secrets"))
}
