# 37. kör jelentése: Antigravity kvótahiba

Claude Code, 2026-11-03, ág: `feature/agy-quota` (a `main` `629fbba`-jéből).

## Commitok

- Egy commit: az okot kiolvasó adapter, a napló-tartalék, az `AgyLimiter`, a szerződés (`ratelimit-note`, `HostReport.agy_limits`), a csendes üzenet szövege, a Gépek oldal, tesztek, dokumentáció.

## Honnan jön a hiba oka (bizonyíték)

- **A `result` esemény `error` mezője.** Szándékosan rossz modellnévvel (`agy -p "say ok" --model no-such-model-xyz --output-format stream-json`, agy 1.4.12) az eredmény:
  `{"event":"result","result":{"status":"ERROR","response":"","error":"invalid model selection (...)", ...}}`.
  Az őr ezt a mezőt eddig nem olvasta, ezért látszott a 11. 01-i esetben üres hibaszöveg.
- **Kvótahibát nem tudtam kiváltani,** így nem láttam, hogy az agy azt is az `error` mezőbe írja-e. A bináris tartalmaz `json:"error"` mezőket és a `Print mode: run ended with error and no response: %v` sort. Ezért a napló a tartalék.
- **A 11. 01-i napló** (`cli-20261101_091403.log`) a `session.go:280` sorban és a `RESOURCE_EXHAUSTED` sorban hordozza az okot. Ugyanebben a naplóban **fiókadat (e-mail-cím) is van**, ezért abból csak a hibasor kerül ki.

## A limiter választása

**Új `AgyLimiter`, nem a `ClaudeLimiter` általánosítása.**
- A Claude fiókszintű, két ablakkal, minden futáskor kapott kihasználtsággal és tartalékkal dolgozik.
- Az agy nem jelent semmit, csak elfogy, és modellenként fogy.
- A közös rész (állapotfájl, újraindulás utáni folytatás, ébresztés a végén, szálanként egy üzenet) megvan az új típusban és a közös `postNotes`-ban. A két eltérő logikát egy típusba gyúrni bonyolultabb lett volna.

## Tesztek

```
go test ./internal/guard/ -run 'Agy|Antigravity'   ok
TestAntigravityKeepsTheErrorField     a stream- és a JSON-kimenet error mezője az okba kerül
TestParseAgyReset                     6h43m54s, 15m, 2h, 30s; idő nélkül nincs
TestAgyLogReason                      valósághű napló zajjal és e-mail-címmel; régebbi napló nem; symlink nem; két eltérő ok: nincs találgatás; RESOURCE_EXHAUSTED-sor tartaléknak
TestAgyQuotaPausesTheModelOnly        nem számít próbálkozásnak (ma elbukna), a tétel függőben, egy üzenet; amíg a kézi óra át nem lépi a visszaállást, nincs futás; másik modell fut; újraindulás után is szünetel
TestAgyQuotaWithoutResetRetriesIn30Minutes
TestAgyAnswerMentioningQuotaDoesNotPause   sikeres válasz „quota reached” szöveggel nem állít meg; a válaszmező sem
TestAgyOtherFailureStillCountsAttempts
TestClaudeLimitsReport/RateLimitNote (e2e)  az új üzenetszöveg, a vezérlőkarakterek kiszűrése, az agy_limits a /hosts válaszban
HostsPage.test.tsx                    a Gépek oldal sorai és jelvénye
make test, make lint, make test-web, race (guard, e2e)   ok
```

## Ami kimaradt

- **Mérés, mielőtt elfogy:** az agy nem ad használati adatot.
- **Automatikus váltás másik modellre:** nem része a körnek.
- **Tömörítő (scribe) futások:** nem állnak meg, csak a hibájuk okát kapják meg.

## Élesítés

A szerver előbb (új ablakértékek a `ratelimit-note`-ban, új mező a jelentésben; migráció nincs), utána az őrök. Ha egy régi őr fut az új szerverrel, nincs gond; ha új őr a régi szerverrel, a csendes üzenet 400-at kap (ezt csak naplózza).

## Amit az átnéző először nézzen meg

1. `agyLogReason`: a fájlnév szerinti időszűrés, a 256 KB-os farok, az `os.Root`, hogy csak a hibasor jön ki.
2. `asAgyQuota`: csak a `Reason` dönt, a `Response` soha.
3. `agyModels`: döntés+munka modellje; közvetlen módban a munkamodell.
