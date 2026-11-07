# iseall

**I see all.** A single-binary, zero-dependency server dashboard for Raspberry Pi (and any Linux box).
One ~8 MB static Go binary serves a live web UI: open it from your laptop or phone, or full-screen on the Pi's own display.

## What's in it

| Page | What you get |
|---|---|
| **Overview** | Live CPU / memory / temperature / disk gauges, 5-minute charts, per-core heat grid, load, Pi under-voltage & throttle flags, alerts, and a launchpad for every web app on the box |
| **Processes** | Every process with CPU, memory, age, command line, and the **Docker container or systemd service it belongs to**. Sort, filter, signal, renice, inspect |
| **Network** | Per-interface live throughput, addresses, Wi-Fi signal, errors/drops, gateway & DNS |
| **Storage** | Filesystems (with a timeout so dead NFS mounts can't hang it), inodes, devices, per-disk I/O |
| **Docker** | Containers grouped by compose stack: start/stop/restart/pause/kill/remove, live CPU sparklines, memory, health, logs, inspect (secrets masked), **shell into the container**, set the restart policy (= start on boot). Stack *Update* (pull + up), images, volumes, networks, prune |
| **Services** | All systemd units with per-service CPU/memory/tasks, start/stop/restart, **start-at-boot toggle**, live journal, unit file |
| **Startup** | **Create your own "run at power-on" programs** (each becomes a supervised systemd service with restart, delay, user, env, wait-for-network), and see *everything* that starts at boot: services, containers, cron `@reboot`, desktop autostart, rc.local, timers, cron jobs, plus a boot-time breakdown |
| **Ports** | Every listening port mapped to its process, container and service, flagged **exposed vs local**, with auto-detected web-UI page titles and *Open* links. Live connections & top remote IPs |
| **Logs** | Live journal with unit / priority / time / regex filters |
| **Terminal** | Real shells in the browser (multiple tabs; they stay alive while you browse) |
| **Security** | Failed SSH logins & top offending IPs, sessions, firewall, sshd posture checks, exposed ports, pending apt updates |
| **Settings** | Alert thresholds, theme, Lite mode, password, reboot/shutdown |

`Ctrl K` opens a command palette that searches pages, containers, services, processes and web UIs, and runs actions.
`g` then a letter jumps between pages, and `/` focuses the filter.

## Install on the Pi

```sh
# on any machine with Go ≥ 1.24 (or on the Pi itself)
./build.sh                         # → dist/iseall-linux-{arm64,armv7,armv6,amd64}

# copy the folder to the Pi, then:
sudo ./install.sh                  # installs + enables iseall.service (starts on every boot)
sudo ./install.sh --kiosk          # …and opens it full-screen on the Pi's display at login
sudo ./install.sh --port 9000      # different port
sudo ./install.sh --uninstall
```

Open `http://<pi-ip>:8765`. The first visit asks you to create a password.
To reset it: `sudo iseall -passwd -config /etc/iseall/config.json`.

## Container image (GHCR)

Every push to `main` runs `.github/workflows/ghcr.yml`. It vets and builds the code, then publishes a multi-arch image (amd64, arm64, arm/v7, arm/v6) to `ghcr.io/<owner>/<repo>` tagged `latest` and `sha-<commit>`.

```sh
docker run -d --name iseall --restart unless-stopped \
  --pid host --network host --privileged \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v iseall-config:/etc/iseall \
  ghcr.io/<owner>/<repo>:latest
```

Host PID and network namespaces let it see real processes, ports and interfaces, and the socket mount enables Docker management. The native install above is still the recommended way to run iseall. Inside a container, **Services, Startup apps, Logs and power actions don't work** (there's no host systemd or journal), and the terminal opens a shell inside the container, not on the host.

## Why it's light

- **Go stdlib only.** No cgo, no frameworks, no Node at runtime. The UI is hand-written JS and CSS, embedded and pre-gzipped (~50 KB over the wire, xterm.js lazy-loaded only for the terminal).
- **Reads the kernel directly.** Stats come from `/proc` and `/sys`, and Docker CPU/memory come from cgroup files instead of the expensive `docker stats` API. Port ownership comes from socket inodes.
- **Samples only when someone's watching.** 1 s while a browser is connected and 5 s otherwise. Heavy views (processes, ports) are computed on demand only while that page is open, and only while the tab is visible.
- **Caches the slow stuff.** Command lines and cgroups once per process lifetime, restart policies, `systemd-analyze`, and web-UI probes.
- ~15–20 MB RSS. The service unit caps it at `MemoryMax=96M`.

## Security notes

iseall runs as **root** (it must, to see every process and socket and to manage services and containers), and its terminal gives a root shell. Treat the password like the root password.

- Passwords are PBKDF2-SHA256. Sessions are HMAC-signed `HttpOnly`/`SameSite=Strict` cookies, and changing the password signs everyone out.
- Login is rate-limited per IP, and every mutating request needs a custom header (CSRF). The WebSocket checks the Origin.
- For access from outside your LAN, put it behind a VPN (Tailscale/WireGuard) rather than port-forwarding it. Optional TLS: `-cert` / `-key`.

## Flags

```
-addr :8765            listen address
-config path           default /etc/iseall/config.json (as root)
-cert / -key           serve HTTPS
-passwd                set the password interactively and exit
-version
```
