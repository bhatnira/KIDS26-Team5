package misc

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if duration, err := time.ParseDuration(s); err == nil {
		return duration, nil
	}

	if !strings.Contains(s, "d") {
		return 0, fmt.Errorf("invalid duration format: %s (must contains units or use standard format)", s)
	}

	dayIndex := strings.Index(s, "d")
	if strings.LastIndex(s, "d") != dayIndex {
		return 0, fmt.Errorf("multiple 'd' in duration: %s", s)
	}

	days, err := strconv.Atoi(s[:dayIndex])
	if err != nil || days < 0 {
		return 0, fmt.Errorf("invalid day value: %s", s[:dayIndex])
	}
	dayDuration := time.Hour * 24 * time.Duration(days)

	remaining := s[dayIndex+1:]
	if remaining == "" {
		return dayDuration, nil
	}

	remainingDuration, err := time.ParseDuration(remaining)
	if err != nil {
		return 0, fmt.Errorf("invalid duration after 'd': %s", remaining)
	}
	return dayDuration + remainingDuration, nil
}
