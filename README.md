# iPyInterVu

iPyInterVu is a Go web server that serves a chat UI and proxies requests to [OpenRouter](https://openrouter.ai/). The OpenRouter API key and auth private key stay on the server; users authenticate with an RSA public key distributed outside the app.

The application code lives in [`openrouter-app/`](openrouter-app/). Deployment scripts are in [`openrouter-app/deploy/`](openrouter-app/deploy/).

## Prerequisites

- A DigitalOcean account (for the easiest setup path below)
- A domain name with DNS you can edit (for example `aalang.org`)

For the alternative scripted setup from a fresh server, you also need:

- Ubuntu 24.04 (or similar Linux)
- Go 1.22+ installed (for building from source)
- An [OpenRouter API key](https://openrouter.ai/)
- An RSA keypair for user authentication (Ed25519 / OpenSSH keys are not supported)

## Secrets and the repository

The OpenRouter API key and `.pem` key files are **not stored in this repository**. On a backup-restored Droplet they already live in `/etc/openrouter-app/`. On a fresh server they are created during scripted setup. Do not commit secrets to git.

---

## Easiest way: Create a Droplet from a DigitalOcean backup (aalang-Web)

If the existing **aalang-Web** Droplet is already configured for iPyInterVu, the fastest way to get a running server is to create a new Droplet from a DigitalOcean **backup** or **snapshot** of that machine. You skip generating keys, installing secrets, building the app, and configuring HTTPS by hand — all of that is already on the disk image.

If you cannot use that backup, skip to [initial setup from a cloned repo](#alternative-initial-setup-from-a-cloned-repo) below.

A backup image includes: secrets in `/etc/openrouter-app/`, the `openrouter-app` systemd service, nginx, Let's Encrypt certificates, and (if present on the source Droplet) a clone of this repository.

### 1. Open the backup for aalang-Web

In the [DigitalOcean control panel](https://cloud.digitalocean.com/):

1. Go to **Droplets**
2. Click the existing **aalang-Web** Droplet
3. Open the **Backups** tab (weekly automatic backups) or **Snapshots** (if you created a manual snapshot)

**What this does:** Locates the disk image taken from the running iPyInterVu server. Use the most recent backup or snapshot.

---

### 2. Create a Droplet from the backup image

1. On the backup or snapshot you want, click **More** → **Create Droplet** (or **Restore** / **Create from snapshot**, depending on the UI)
2. Choose the same **region** as the original Droplet if possible (lowest latency, same VPC if you use one later)
3. Choose a **size** (same or larger than aalang-Web)
4. Add your **SSH key**
5. Set a hostname (for example `aalang-web-2` or keep `aalang-Web` if replacing the old Droplet)
6. Click **Create Droplet**

**What this does:** Provisions a new Droplet whose disk is a copy of aalang-Web at backup time. The new machine starts with iPyInterVu already installed.

---

### 3. Note the new public IP address

When the Droplet finishes creating, copy its **public IPv4 address** from the Droplet page.

**What this does:** The new Droplet almost always gets a different IP than the original. You need this address for DNS.

---

### 4. Update DNS for your domain

At your DNS provider, update the **A record** for `aalang.org` (and `www.aalang.org` if used) to point to the **new** Droplet IP.

Wait for DNS to propagate (minutes to hours).

**What this does:** Sends browser traffic for `https://aalang.org/` to the new Droplet instead of the old one.

---

### 5. SSH into the new Droplet and verify services

```bash
ssh manager@<new-droplet-ip>

sudo systemctl status openrouter-app
sudo systemctl status nginx
curl http://127.0.0.1:8080/healthz
curl https://aalang.org/healthz
```

Expected: both health checks return `ok`; `openrouter-app` and `nginx` are **active**.

**What this does:** Confirms the restored image booted correctly and the app and reverse proxy are running.

---

### 6. Renew or re-issue TLS if HTTPS fails

If `curl https://aalang.org/healthz` fails with a certificate error after the IP change, renew Let's Encrypt on the new Droplet:

```bash
sudo certbot renew
sudo systemctl reload nginx
```

If renewal fails, re-run certbot for the domain:

```bash
sudo certbot --nginx -d aalang.org
sudo systemctl reload nginx
```

**What this does:** Refreshes the TLS certificate for the new server. Certificates are stored on disk and are usually restored with the backup; renewal may still be needed after an IP or hostname change.

---

### 7. Update application code (optional)

If the repo is cloned on the Droplet and you need changes made after the backup was taken:

```bash
cd /home/manager/iPyIntervuServer
git pull

cd openrouter-app
./deploy/redeploy.sh
```

**What this does:** Rebuilds and restarts the Go app from the latest code without re-running secrets or HTTPS setup.

---

### 8. Retire or destroy the old Droplet (optional)

After DNS points at the new Droplet and `https://aalang.org/` works:

1. Confirm the new server handles traffic for at least a few hours
2. Power off or destroy the old **aalang-Web** Droplet in DigitalOcean to avoid paying for two servers

**What this does:** Completes migration to the new Droplet. Do not destroy the old Droplet until the new one is verified.

---

### 9. Use the application

Open `https://aalang.org/` in a browser.

1. Paste the public key when prompted (first visit only, unless you already have it in browser storage)
2. Select a model from the dropdown
3. Send a chat message

**What this does:** Confirms end-to-end access. Users need the public key distributed out-of-band (same key as before if you restored from the same aalang-Web backup).

You do **not** need to run `install-secrets.sh`, `install.sh`, or `setup-https.sh` on a successfully restored Droplet unless something is missing or broken.

**No access to the aalang-Web backup?** Follow the [initial setup instructions](#alternative-initial-setup-from-a-cloned-repo) below instead.

---

## Alternative: Initial setup from a cloned repo

Use this path if you **do not have access** to a DigitalOcean backup or snapshot of **aalang-Web**, or if you are starting from a **fresh Ubuntu Droplet** with no prior iPyInterVu configuration.

Assume the repo is cloned to the server, for example:

```text
/home/manager/iPyIntervuServer/
```

All commands below use:

```bash
cd /home/manager/iPyIntervuServer/openrouter-app
```

Adjust the path if your clone lives elsewhere.

### 1. Generate an RSA keypair

Create an RSA private/public keypair on the server (or on a trusted machine, then copy the private key to the server).

```bash
mkdir -p env
openssl genpkey -algorithm RSA -out env/ipyintervu-key.pem -pkeyopt rsa_keygen_bits:2048
openssl rsa -in env/ipyintervu-key.pem -pubout -out env/ipyintervu-pub.pem
chmod 600 env/ipyintervu-key.pem
```

**What this does:** Creates the keypair used for challenge–response authentication. The private key stays on the server. The public key (`env/ipyintervu-pub.pem`) is given to users out-of-band; they paste it into the web UI on first visit.

---

### 2. Install secrets on the server

```bash
sudo ./deploy/install-secrets.sh
```

Optional: pass a custom path to the private key PEM:

```bash
sudo ./deploy/install-secrets.sh /path/to/ipyintervu-key.pem
```

**What this does:**

- Creates `/etc/openrouter-app/`
- Creates the `openrouter` system user (if missing)
- Prompts for your OpenRouter API key and writes `/etc/openrouter-app/env`
- Copies the private key to `/etc/openrouter-app/auth-private-key.pem`
- Sets restrictive file permissions (`env` → `600`, private key → `640`)

If secrets already exist, this script does not overwrite them.

---

### 3. Build and install the application

```bash
sudo ./deploy/install.sh
```

**What this does:**

- Verifies secrets from step 2 are present
- Runs `deploy/build.sh` to compile a Linux `amd64` binary
- Installs the binary to `/usr/local/bin/openrouter-app`
- Installs the systemd unit (`openrouter-app.service`)
- Enables and starts the `openrouter-app` service on boot
- Binds the app to `127.0.0.1:8080` (localhost only)

Verify:

```bash
curl http://127.0.0.1:8080/healthz
```

Expected output: `ok`

At this point the app runs, but it is only reachable locally. Remote browsers need HTTPS for authentication (Web Crypto).

---

### 4. Enable HTTPS with nginx and Let's Encrypt

Your domain must already resolve to this server's public IP.

```bash
sudo ./deploy/setup-https.sh <your-domain> [email]
```

Example:

```bash
sudo ./deploy/setup-https.sh aalang.org admin@aalang.org
```

**What this does:**

- Installs nginx, certbot, and the certbot nginx plugin
- Configures nginx to reverse-proxy to `127.0.0.1:8080`
- Obtains a Let's Encrypt TLS certificate for your domain
- Redirects HTTP to HTTPS
- Reloads nginx

Verify:

```bash
curl https://<your-domain>/healthz
```

Expected output: `ok`

---

### 5. Distribute the public key to users

Give users the contents of `env/ipyintervu-pub.pem` outside the web app (email, document, in person, etc.).

**What this does:** Users need the public key to authenticate. On first visit to `https://<your-domain>/`, they paste the key into the prompt. The browser stores it in `localStorage` for later visits.

---

### 6. Use the application

Open `https://<your-domain>/` in a browser.

1. Paste the public key when prompted (first visit only)
2. Select a model from the dropdown
3. Send a chat message

---

## Choosing a setup path

| Approach | Use when |
|----------|----------|
| **Backup / snapshot** (easiest; listed first above) | You have access to a backup or snapshot of the configured aalang-Web Droplet |
| **Initial setup from cloned repo** (alternative above) | You do not have access to that backup, or you are on a fresh Ubuntu Droplet |

---

## Updating the application

After code changes or `git pull`, rebuild and restart without repeating the full install:

```bash
cd /home/manager/iPyIntervuServer/openrouter-app
./deploy/redeploy.sh
```

**What this does:**

- Runs `deploy/build.sh`
- Replaces `/usr/local/bin/openrouter-app` with the new binary
- Restarts the `openrouter-app` systemd service

nginx and secrets are not changed.

---

## Deploy scripts reference

| Script | When to run | Requires root |
|--------|-------------|---------------|
| `deploy/build.sh` | Compile Linux binary only | No |
| `deploy/install-secrets.sh` | First-time secret setup (scripted path only) | Yes |
| `deploy/install.sh` | First-time app + systemd install (scripted path only) | Yes |
| `deploy/setup-https.sh` | First-time HTTPS setup (scripted path only) | Yes |
| `deploy/redeploy.sh` | Code updates after install | Partial (`sudo` for install/restart) |
| `deploy/fix-nginx-proxy.sh` | Fix nginx proxying to wrong backend port | Yes |

---

## Troubleshooting

**`curl http://127.0.0.1:8080/healthz` fails**

- Check the service: `sudo systemctl status openrouter-app`
- On a fresh server, ensure scripted setup steps 2 and 3 completed
- On a restored Droplet, try `sudo systemctl restart openrouter-app`

**Browser shows "Connection is not private"**

- Use `https://<your-domain>/` (not `http://` or a bare IP)
- Avoid `www.` unless the certificate includes that name
- Try a private browser window to clear cached TLS state

**Authentication fails in the browser**

- HTTPS is required for remote users
- Confirm the public key matches the server's private key
- Use `https://<your-domain>/`, not `http://<public-ip>/`

**`502 Bad Gateway` from nginx**

- The Go app must be running on `127.0.0.1:8080`
- Run `sudo ./deploy/fix-nginx-proxy.sh <your-domain>` if an old nginx config proxies to the wrong port

---

## Further reading

- [`design.md`](design.md) — full architecture and phased deployment plan
- [`digitalocean-openrouter-go-proposal.md`](digitalocean-openrouter-go-proposal.md) — original deployment proposal
