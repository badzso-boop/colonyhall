// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package setup

import (
	"fmt"
	"os"
	"strings"
)

// Lang is the installer's language: English by default, Hungarian when the
// locale says so.
type Lang string

// Languages.
const (
	English   Lang = "en"
	Hungarian Lang = "hu"
)

// DetectLang reads the POSIX locale variables in their order of precedence.
func DetectLang(getenv func(string) string) Lang {
	if getenv == nil {
		getenv = os.Getenv
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := getenv(k); v != "" {
			if strings.HasPrefix(strings.ToLower(v), "hu") {
				return Hungarian
			}
			return English
		}
	}
	return English
}

// T translates a message key; a missing key shows itself (the test checks
// that every key has both languages).
func (l Lang) T(key string, args ...any) string {
	m, ok := messages[key]
	if !ok {
		return key
	}
	s := m[0]
	if l == Hungarian {
		s = m[1]
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// messages: key → {English, Hungarian}.
var messages = map[string][2]string{
	// general
	"welcome":      {"Colonyhall setup %s (%s)", "Colonyhall telepítő %s (%s)"},
	"dryRun":       {"Dry run: nothing is changed. The plan:", "Próbafuttatás: semmi nem változik. A terv:"},
	"stepDone":     {"✓ %s: already done", "✓ %s: már kész"},
	"stepApply":    {"→ %s", "→ %s"},
	"stepOK":       {"✓ %s", "✓ %s"},
	"stepFailed":   {"✗ %s: %v", "✗ %s: %v"},
	"resume":       {"Fix the problem and run `colony setup` again: it continues where it stopped. Log: %s", "Javítsd a hibát, és futtasd újra a `colony setup`-ot: onnan folytatja, ahol megállt. Napló: %s"},
	"willRun":      {"This will run:", "Ez fog futni:"},
	"proceed":      {"Go ahead?", "Mehet?"},
	"skipped":      {"Skipped: %s", "Kihagyva: %s"},
	"mode":         {"What should this machine be?", "Mi legyen ez a gép?"},
	"modeServer":   {"the server (the web app and the database)", "a szerver (a webes felület és az adatbázis)"},
	"modeGuard":    {"a guard (runs AI agents for an existing server)", "őr (AI-agenteket futtat egy meglévő szerverhez)"},
	"modeBoth":     {"both (a server, and agents on this machine too)", "mindkettő (szerver, és ezen a gépen is futnak agentek)"},
	"finished":     {"Done.", "Kész."},
	"answersPerms": {"Warning: the answers file %s can be read by others; it may hold secrets (chmod 600 it).", "Figyelem: a(z) %s válaszfájlt mások is olvashatják; titkot tartalmazhat (chmod 600)."},

	// dependencies
	"depsTitle":     {"Programs this needs", "Szükséges programok"},
	"depsMissing":   {"Missing: %s", "Hiányzik: %s"},
	"depsInstall":   {"Install them now with %s?", "Telepítsem most őket ezzel: %s?"},
	"depsManual":    {"Install these yourself, then run `colony setup` again: %s", "Ezeket telepítsd magad, majd futtasd újra a `colony setup`-ot: %s"},
	"depsNoManager": {"No supported package manager found (apt, dnf, brew).", "Nincs támogatott csomagkezelő (apt, dnf, brew)."},
	"depsDocker":    {"Docker is needed for the server. On macOS install Docker Desktop (https://www.docker.com/products/docker-desktop/) or OrbStack (https://orbstack.dev), start it, and run `colony setup` again.", "A szerverhez Docker kell. macOS-en telepítsd a Docker Desktopot (https://www.docker.com/products/docker-desktop/) vagy az OrbStacket (https://orbstack.dev), indítsd el, és futtasd újra a `colony setup`-ot."},

	// server
	"srvDir":           {"Server folder", "A szerver mappája"},
	"srvDB":            {"Database", "Adatbázis"},
	"srvDBOwn":         {"its own Postgres in Docker (recommended)", "saját Postgres Dockerben (ajánlott)"},
	"srvDBExternal":    {"an existing Postgres (a connection URL)", "meglévő Postgres (kapcsolati URL)"},
	"srvDBURL":         {"Postgres URL (postgres://user:password@host:5432/db), not shown", "Postgres URL (postgres://felhasználó:jelszó@gép:5432/adatbázis), nem látszik"},
	"srvDBLocalhost":   {"Note: inside the container, localhost is the container itself; use the host's address or a Docker network name.", "Megjegyzés: a konténerben a localhost maga a konténer; a gép címét vagy egy Docker-hálózat nevét add meg."},
	"srvBind":          {"Who may reach the server?", "Ki érje el a szervert?"},
	"srvBindLocal":     {"only this machine (127.0.0.1, safest)", "csak ez a gép (127.0.0.1, a legbiztonságosabb)"},
	"srvBindLAN":       {"this machine's local network address %s", "a gép helyi hálózati címe: %s"},
	"srvBindTS":        {"this machine's Tailscale address %s", "a gép Tailscale-címe: %s"},
	"srvBindAll":       {"every network interface (0.0.0.0)", "minden hálózati csatoló (0.0.0.0)"},
	"srvBindAllWarn":   {"0.0.0.0 makes the server reachable on every network this machine is on, including public ones. There is no HTTPS: put it behind a reverse proxy with TLS first. Really bind to 0.0.0.0?", "A 0.0.0.0-val a szerver minden hálózatról elérhető, amin a gép van, a nyilvánosakról is. HTTPS nincs: előbb tedd egy TLS-es fordított proxy mögé. Biztosan 0.0.0.0?"},
	"srvPort":          {"Port", "Port"},
	"srvAdminHandle":   {"The first admin's (owner's) user name", "Az első admin (tulajdonos) felhasználóneve"},
	"srvAdminName":     {"Their display name", "A megjelenített neve"},
	"srvAdminPass":     {"Their password (at least 12 characters, not shown; empty: generate one)", "A jelszava (legalább 12 karakter, nem látszik; üresen: generálok egyet)"},
	"srvAdminPass2":    {"The password again", "A jelszó még egyszer"},
	"srvPassMismatch":  {"The two passwords differ, or it is shorter than 12 characters.", "A két jelszó eltér, vagy rövidebb 12 karakternél."},
	"srvPassGenerated": {"The owner's generated password (shown only now, save it): %s", "A tulajdonos generált jelszava (csak most látszik, mentsd el): %s"},
	"srvBackups":       {"Back up the database and files every night into %s (14 days)?", "Mentsem minden éjjel az adatbázist és a fájlokat ide: %s (14 nap)?"},
	"srvReady":         {"The server runs: %s", "A szerver fut: %s"},
	"srvWaiting":       {"Waiting for the server to start…", "Várom, hogy a szerver elinduljon…"},
	"srvNoTLS":         {"There is no HTTPS: keep the server on this machine, a home network or Tailscale, or put it behind a reverse proxy with TLS (docs/16-telepites.md).", "Nincs HTTPS: maradjon a szerver ezen a gépen, otthoni hálózaton vagy Tailscale-en, vagy tedd TLS-es fordított proxy mögé (docs/16-telepites.md)."},
	"srvSudoDocker":    {"Docker commands run with sudo (being in the docker group is root-equivalent).", "A Docker-parancsok sudo-val futnak (a docker csoport tagsága root-egyenértékű)."},
	"srvOpen":          {"Open %s in a browser and sign in as @%s.", "Nyisd meg böngészőben: %s, és lépj be @%s-ként."},

	// guard
	"grdServer":       {"The server's address (for example http://127.0.0.1:8080)", "A szerver címe (például http://127.0.0.1:8080)"},
	"grdCode":         {"The pairing code (web app: Machines → New machine)", "A párosító kód (webes felület: Gépek → Új gép)"},
	"grdName":         {"This machine's name in Colonyhall", "A gép neve a Colonyhallban"},
	"grdRoots":        {"Folders agents may be given (comma-separated)", "Mappák, amelyeket agentek kaphatnak (vesszővel elválasztva)"},
	"grdRootsWSL":     {"The Windows drives (/mnt/c …) are not offered: they are slow from WSL and Windows permissions apply to them. Add one only if you need it.", "A Windows-meghajtókat (/mnt/c …) nem ajánlom fel: WSL-ből lassúak, és a Windows-jogosultságok érvényesek rájuk. Csak akkor add hozzá, ha kell."},
	"grdFull":         {"Allow full-access agents (no sandbox, your own login and settings, everything this user can do)?", "Engedélyezed a teljes hozzáférésű agenteket (sandbox nélkül, a saját belépéseddel és beállításaiddal, mindent megtehetnek, amit ez a felhasználó)?"},
	"grdNoSandboxMac": {"macOS has no sandbox yet: only full-access agents run on this machine; the server refuses sandboxed ones here.", "macOS-en még nincs sandbox: ezen a gépen csak teljes hozzáférésű agent futhat; a szerver itt elutasítja a sandboxosokat."},
	"grdMax":          {"How many agent runs at once on this machine (0: no limit)", "Egyszerre hány agentfutás legyen ezen a gépen (0: korlátlan)"},
	"grdEnrolled":     {"The machine is paired.", "A gép párosítva."},
	"grdService":      {"Start the guard automatically (a %s service)?", "Induljon az őr automatikusan (%s szolgáltatás)?"},
	"grdLinger":       {"Keep it running when you are logged out (loginctl enable-linger, needs sudo)?", "Fusson akkor is, ha nem vagy bejelentkezve (loginctl enable-linger, sudo kell)?"},
	"grdWSLSystemd":   {"WSL runs without systemd here. Enable it in /etc/wsl.conf (needs sudo)? Afterwards run `wsl --shutdown` in Windows, open the terminal again and run `colony setup` again.", "Itt a WSL systemd nélkül fut. Bekapcsoljam az /etc/wsl.conf-ban (sudo kell)? Utána Windowsban `wsl --shutdown`, nyisd újra a terminált, és futtasd újra a `colony setup`-ot."},
	"grdWSLRestart":   {"systemd is enabled. Now run `wsl --shutdown` in Windows (PowerShell or cmd), open this terminal again and run `colony setup` again.", "A systemd be van kapcsolva. Most Windowsban (PowerShell vagy cmd) futtasd: `wsl --shutdown`, nyisd újra ezt a terminált, és futtasd újra a `colony setup`-ot."},
	"grdKeepalive":    {"Keep this WSL distribution running after you log in to Windows (a Windows logon task)? Without it the guard runs only while a WSL terminal is open.", "Fusson ez a WSL-disztribúció a Windows-bejelentkezés után is (Windows-feladat bejelentkezéskor)? Nélküle az őr csak addig fut, amíg egy WSL-terminál nyitva van."},
	"grdNoKeepalive":  {"The guard runs only while a WSL terminal is open.", "Az őr csak addig fut, amíg egy WSL-terminál nyitva van."},
	"grdColleague":    {"A colleague on this machine with their own agents and subscription: create a separate Linux user for them and run `colony setup --guard` as that user.", "Kolléga ezen a gépen saját agentekkel és előfizetéssel: hozz létre neki külön Linux-felhasználót, és azzal futtasd a `colony setup --guard`-ot."},
	"grdNextAgent":    {"Next: in the web app open Agents → New agent, choose this machine and a folder.", "Következő lépés: a webes felületen Agentek → Új agent, válaszd ezt a gépet és egy mappát."},

	// AI clients
	"cliPick":      {"Which AI clients should run agents here? (claude, agy; comma-separated, empty: none)", "Mely AI-kliensek futtassanak itt agenteket? (claude, agy; vesszővel, üresen: egyik sem)"},
	"cliMissing":   {"%s is not installed.", "A(z) %s nincs telepítve."},
	"cliInstaller": {"%s's installer: %s (%d bytes, SHA-256 %s)", "A(z) %s telepítője: %s (%d bájt, SHA-256 %s)"},
	"cliShow":      {"Show the installer script before running it?", "Megmutassam a telepítőszkriptet futtatás előtt?"},
	"cliRun":       {"Run %s's installer now?", "Futtassam most a(z) %s telepítőjét?"},
	"cliNoAgy":     {"Antigravity's installer is not part of this setup yet: install agy yourself, then run `colony doctor`.", "Az Antigravity telepítése még nincs a telepítőben: telepítsd az agy-t magad, majd futtasd a `colony doctor`-t."},
	"loginOK":      {"%s: logged in", "%s: bejelentkezve"},
	"loginToken":   {"%s: a login token set from the web app is in use", "%s: a webes felületről beállított belépési token van használatban"},
	"loginClaude":  {"Claude Code is not logged in on this machine. As this user run `claude` and sign in, or give the web app your own subscription's token: Machines → Claude login (`claude setup-token` on your own computer). Then run `colony doctor`. Colonyhall never logs in for you and never asks for your AI account password.", "A Claude Code nincs bejelentkezve ezen a gépen. Ezzel a felhasználóval futtasd a `claude`-ot és lépj be, vagy add meg a saját előfizetésed tokenjét a webes felületen: Gépek → Claude-belépés (`claude setup-token` a saját gépeden). Utána `colony doctor`. A Colonyhall soha nem lép be helyetted, és soha nem kéri az AI-fiókod jelszavát."},
	"loginAgy":     {"Antigravity (agy) is not logged in on this machine. As this user run `agy` and sign in, then run `colony doctor`.", "Az Antigravity (agy) nincs bejelentkezve ezen a gépen. Ezzel a felhasználóval futtasd az `agy`-t és lépj be, majd `colony doctor`."},
}
