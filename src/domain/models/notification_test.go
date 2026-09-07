package models

import "testing"

func TestNotificationLevelValid(t *testing.T) {
	valid := []NotificationLevel{
		NotificationLevelInfo,
		NotificationLevelSuccess,
		NotificationLevelWarning,
		NotificationLevelError,
	}
	for _, l := range valid {
		if !l.Valid() {
			t.Errorf("%q should be valid", l)
		}
	}
	for _, l := range []NotificationLevel{"", "bogus", "INFO", "critical"} {
		if l.Valid() {
			t.Errorf("%q should be invalid", l)
		}
	}
}
