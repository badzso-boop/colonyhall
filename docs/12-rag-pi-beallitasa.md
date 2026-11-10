# A szöveg-beágyazó a Raspberry Pi 3B-n (36. kör)

Állapot: **terv, jóváhagyásra vár** (a `11-rag-terv.md` része). Norbi futtatja, vagy Claude Code az ő jóváhagyásával, lépésenként. A beágyazó szerver kódja (`deploy/embed/`) a 36a első részében készül; addig ez a lépéssor a gép előkészítése.

Mit ad: egy mindig futó, kicsi HTTP-szolgáltatást a LAN-on, ami magyar (és bármilyen) szövegből 384 dimenziós vektort számol a `multilingual-e5-small` modellel. Csak a homelab (a Colonyhall szerver) hívja.

## 1. Operációs rendszer

- **Raspberry Pi OS Lite, 64 bites** (Bookworm vagy újabb; az ONNX Runtime-nak `aarch64` kell, a 32 bites rendszeren nem fut). Raspberry Pi Imager: gépnév `colony-embed`, SSH kulccsal, jelszavas SSH kikapcsolva, felhasználó `embed`.
- Fix IP a routerben (DHCP-foglalás), pl. `192.168.1.120`. A továbbiakban `PI_IP`.
- Frissítés: `sudo apt update && sudo apt full-upgrade -y && sudo reboot`.
- Jó minőségű, legalább A1-es SD-kártya és **a hivatalos 2,5 A-es táp** (gyenge tápnál a Pi 3B terhelés alatt lassít vagy újraindul: `vcgencmd get_throttled` → `0x0` legyen).

## 2. Memória és az SD-kártya kímélése

A Pi 3B-nek 1 GB RAM-ja van; a modell és a futtatókörnyezet a PC-n mérve ~750 MB csúcsig megy (a Pi-n ⏳ mérés hátravan), ezért kell a zram, és a szerver legfeljebb 8 bemenetet számol egyszerre.

- **zram-swap** az SD-kártyás swap helyett: `sudo apt install -y zram-tools`, `/etc/default/zramswap`: `ALGO=zstd`, `PERCENT=50`; `sudo systemctl disable --now dphys-swapfile` (az SD-kártyára nem swapolunk).
- **Napló memóriában:** `/etc/systemd/journald.conf`: `Storage=volatile`, `RuntimeMaxUse=32M`. (Újraindításkor a napló elvész; a szolgáltatás hibáit a Colonyhall állapotlapja mutatja.)
- `noatime` a gyökér fájlrendszeren (`/etc/fstab`).
- GPU-memória minimumra: `/boot/firmware/config.txt`: `gpu_mem=16` (nincs kijelző).

## 3. A beágyazó szolgáltatás

```sh
sudo apt install -y python3-venv
sudo mkdir -p /opt/colony-embed && sudo chown embed: /opt/colony-embed
cd /opt/colony-embed
python3 -m venv venv
# a verziók a deploy/embed/requirements.txt-ből, rögzítve és hash-sel:
venv/bin/pip install --require-hashes -r requirements.txt
```

- A szerver (`deploy/embed/server.py`) és a `requirements.txt` a repóból kerül ide (`scp`).
- **A modell:** az ONNX int8 fájl és a tokenizer a `deploy/embed/MODEL.md`-ben rögzített Hugging Face-commitról, sha256-tal ellenőrizve; `/opt/colony-embed/model/` (csak olvasható: `chmod -R a-w`). A modellnév a válaszokban `multilingual-e5-small-int8@<sha256 első 12 jele>`.
- **systemd** (`/etc/systemd/system/colony-embed.service`):

```ini
[Unit]
Description=Colonyhall text embedding server
After=network-online.target
Wants=network-online.target

[Service]
User=embed
WorkingDirectory=/opt/colony-embed
ExecStart=/opt/colony-embed/venv/bin/python server.py --model model --host 0.0.0.0 --port 8090 --threads 4
Restart=always
RestartSec=5
MemoryMax=900M
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes

[Install]
WantedBy=multi-user.target
```

`sudo systemctl daemon-reload && sudo systemctl enable --now colony-embed`

## 4. Tűzfal: csak a homelab

```sh
sudo apt install -y ufw
sudo ufw default deny incoming
sudo ufw allow from 192.168.1.0/24 to any port 22 proto tcp
sudo ufw allow from 192.168.1.101 to any port 8090 proto tcp   # homelab (szmajli)
sudo ufw enable
```

A szolgáltatásnak nincs tokene: a hálózat szűri (lásd a döntések D5-öt a `11-rag-terv.md`-ben).

## 5. Ellenőrzés

A homelabról:

```sh
curl -s http://PI_IP:8090/healthz
curl -s http://PI_IP:8090/v1/embeddings -H 'Content-Type: application/json' \
  -d '{"model":"multilingual-e5-small","input":["query: hol fut a szerver?"]}' | head -c 200
```

Elvárt: `{"ok":true,"model":"multilingual-e5-small-int8@…"}`, illetve egy 384 elemű vektor; a rövid kérdés ideje ⏳ (mérés hátravan; ide kerül az első méréskor).

A Colonyhall szerver `deploy/.env`-jében: `COLONY_EMBED_URL=http://PI_IP:8090`.

## 6. Újraindítás, frissítés, hiba

- Áramszünet után a szolgáltatás magától indul; a Colonyhall addig kulcsszavas módban keres, és a be nem ágyazott darabokat utána pótolja.
- **Modellcsere** csak a Colonyhall-oldali `rag-reembed`-del együtt (a régi vektorok különben nem összevethetők); a modellnév eltérését a szerver észreveszi, és nem ír vegyes vektort.
- Ha a Pi lassú: `vcgencmd measure_temp` (80 °C fölött lassít: hűtőborda), `vcgencmd get_throttled`, `free -m`.
