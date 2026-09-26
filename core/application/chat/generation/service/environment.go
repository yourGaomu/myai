package service

import (
	"fmt"
	"strings"
	"time"
)

var weekdayNames = [...]string{
	"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday",
}

var weekdayNamesZH = [...]string{
	"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六",
}

func environmentContextPrompt(now time.Time, workspaceRoot string) string {
	if now.IsZero() {
		now = time.Now()
	}
	zone, offset := now.Zone()
	if zone == "" {
		zone = "UTC"
	}
	weekday := now.Weekday()
	prompt := fmt.Sprintf(
		"Current time: %s, %s %s (UTC%+d, %s).",
		weekdayNames[weekday],
		now.Format("2006-01-02 15:04:05"),
		zone,
		offset/3600,
		weekdayNamesZH[weekday],
	)
	if root := strings.TrimSpace(workspaceRoot); root != "" {
		prompt += "\nWorkspace: " + root
	}
	return prompt
}
