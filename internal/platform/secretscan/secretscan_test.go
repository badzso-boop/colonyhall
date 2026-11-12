package secretscan

import "testing"

func TestFlags(t *testing.T) {
	for name, s := range map[string]string{
		"colonyhall":  "the token is col_dev_admin_token_do_not_use_in_production",
		"anthropic":   "export ANTHROPIC_API_KEY=sk-ant-api03-abcdefghijklmnopqrstuvwxyz012345",
		"github":      "ghp_0123456789abcdefghijklmnopqrstuvwxyzAB",
		"aws":         "key AKIAIOSFODNN7EXAMPLE here",
		"private key": "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXk=",
		"jwt":         "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		"url":         "postgres://colonyhall_app:Xy7Pq2Lm9Rt4@100.64.0.20:5432/colonyhall",
		"assigned":    "COLONY_GITEA_WEBHOOK_SECRET=9f86d081884c7d659a2feaa0c55ad015",
		"password":    `"password": "Tr0ub4dor3xyzQ"`,
	} {
		if !Contains(s) {
			t.Errorf("%s: not flagged: %q", name, s)
		}
	}
}

func TestDoesNotFlagOrdinaryText(t *testing.T) {
	for _, s := range []string{
		"A szerver a homelabon fut, a jelszót a deploy/.env-ben tartjuk.",
		"Set COLONY_GITEA_TOKEN=${COLONY_GITEA_TOKEN} in the compose file.",
		"password: <redacted>",
		"the api_key: example-key-value",
		"postgres://user:pass@localhost/db is the example URL",
		"func parseAgyReset(s string) (time.Time, error)",
		"max_concurrent_runs: 2",
		"sk-learn is a library",
		"https://gitea.example.com/alice/colonyhall/pulls/12",
	} {
		if r := Find(s); r != "" {
			t.Errorf("flagged by %s: %q", r, s)
		}
	}
}
