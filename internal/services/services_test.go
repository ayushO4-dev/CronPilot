package services

import "testing"

func TestValidUnitName(t *testing.T) {
	valid := []string{
		"nginx.service", "getty@tty1.service", "user@1000.service",
		"my-app.service", "dbus.service", "systemd-journald.service",
	}
	for _, n := range valid {
		if !ValidUnitName(n) {
			t.Errorf("expected valid: %q", n)
		}
	}

	invalid := []string{
		"", "nginx", "nginx.socket", "nginx.timer",
		"--force.service", "-x.service", "../etc.service",
		"a b.service", "nginx.service;rm -rf /", "evil$(id).service",
		"nginx.service\n", "name/with/slash.service",
	}
	for _, n := range invalid {
		if ValidUnitName(n) {
			t.Errorf("expected invalid: %q", n)
		}
	}
}
