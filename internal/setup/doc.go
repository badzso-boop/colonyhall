// Package setup is `colony setup`, `colony doctor`, `colony upgrade` and
// `colony uninstall` (round 41, docs/15-telepites-terv.md): it installs the
// Colonyhall server (Docker Compose) and the guard on a machine. Every
// outside command runs behind a Runner, so the tests check the plan and the
// command sequence without touching the machine.
package setup
