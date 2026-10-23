# iPyTutor

iPyTutor is a weekly Python tutor that prepares students for **iPyIntervu**, the AI interview that rates them *Not Ready Yet*, *Competent* or *Exceptional* on each week's key concept. It started as a copy of iPyIntervuServer: it keeps that project's login, deployment, week materials and rubrics, and replaces the interviewer with a tutor.

Students can study **any week and any lesson, in any order, at any time**: pick a week from the menu or a lesson from the list, or type `week 6`, `lesson 3`, `week 7 lesson 2` or a topic such as `lists`. The tutor assumes they know everything from the weeks before the one they choose. It keeps **no record of students**: there are no accounts, and nothing is saved between visits.

In any week a student can:

- **Learn the concept** through short lessons (📖 *Teach me*, *Next lesson*, or any lesson by number), with examples set in their major if they give it, and a quick check at the end of each.
- **Practice the interview's three parts** on request, in the interview's own formats:
  - 💬 **Concept questions**: a real-world situation and a question about how the week's Python works, with an interviewer-style follow-up.
  - 🧩 **Code problems**: a story problem with *Data available* and *What's wanted*. The student breaks it down, writes and pastes the code, then explains it and how they used AI.
  - 🐞 **Bug hunts**: a short program with one real bug. The student explains how they'd track it down, then pins down the fix.
  - Week 1 is decomposition only, like its interview.
- **Get their answers evaluated, with interview advice.** Every practice answer gets feedback, an **Interview check** (the rating it would likely get, and what would raise it), and an **Interview prep** tip: one concrete thing to practice or do differently before the interview. Lesson quick checks, worked solutions and explanations in conversation get the same evaluation and advice. Students can ask for a **hint** (progressively more specific), the worked **solution**, or **another one**, **a harder one**, or **an easier one**.
- **See how the interview works** (🎯 *Interview guide*): what each part asks and the week's rubric criteria for Competent and Exceptional.

Students can also just ask questions or paste code to have it explained.

The application lives in [`openrouter-app/`](openrouter-app/), deployment scripts in [`openrouter-app/deploy/`](openrouter-app/deploy/).

## How it works

| Piece | Where |
|---|---|
| Login (RSA challenge and session token, unchanged from iPyIntervu) | `auth.go`, `session.go`, `context.go`, `keys.go` |
| Week materials: the interview's key concepts, competency guides and rubrics | `env/IPYIntervu_support_files/`, `env/rubrics/` |
| Lesson plans, one file per week (`##` heading = lesson, bullets = points to cover) | `env/tutor/weekN_lessons.md` |
| Bug-hunt guide: suitable bugs per week and what good debugging looks like at each level | `env/tutor/bug_hunting_guide.md` |
| Loads and parses all of the above | `curriculum.go` |
| What each week may use, and the check that keeps problems in scope | `week_scope.go`, `week_map.go` |
| Turn routing: week and lesson choice, commands, lessons, practice stages, hints, solutions | `tutor_handler.go`, `tutor_intents.go`, `tutor_session.go` |
| Prompts | `tutor_prompts.go` |
| Practice problems: hidden answer key, scope and format checks, background preparation | `tutor_practice.go` |
| Server-written messages (welcome, week overview, interview guide, help) | `tutor_messages.go` |
| Page state for the sidebar and buttons | `tutor_state.go` |
| OpenRouter client | `llm.go` |
| Browser UI; colors (with the reasoning behind them at the top of the stylesheet) and the parrot logo | `static/`, `static/style.css`, `static/parrot.svg` |

A few design points:

- **Practice problems carry a hidden answer key.** The model writes each problem with a `REFERENCE` section, which the server keeps and the student never sees. Feedback, hints and solutions are judged against it.
- **Problems are checked before students see them.** A problem that uses something from a later week (the same scope checker the interview uses), or that breaks its format (code in a code problem, a bug hunt without a program), is rewritten once.
- **Problems are prepared in the background.** Once a student starts working in a week, the tutor drafts one problem of each kind with reasoning turned on, which makes bug hunts more reliable. A problem is usually ready the moment it's asked for, and the next one is prepared as each is used. Opening the page alone costs no model calls.
- **Each problem moves through the interview's steps.** Feedback ends with a hidden `LEVEL:` line that the server strips; a rated answer moves the problem to its next step (a code problem goes breakdown, code, explanation). A question or a request for help isn't rated and leaves the problem where it was.
- **Nothing is kept between visits.** A session lives in server memory only. It remembers the week, the major, the current lesson or problem and recent problem titles (so problems don't repeat) until the page is reloaded.
- **Fixed text comes from the server.** Welcome, week overview, interview guide and help need no model call, so they're instant. If the model is unreachable, a lesson falls back to its outline.

### Editing the curriculum

- **Lessons:** edit `env/tutor/weekN_lessons.md`. Keep each lesson to what students have learned by that week; `go test` flags later-week ideas (for example, the word "file" before Week 9).
- **Rubrics and week materials:** copy updated files from iPyIntervuServer over the ones in `env/`, so the tutor teaches to what the interview grades.
- **Model:** set `TUTOR_MODEL` (default `deepseek/deepseek-v4-flash`, which is DeepSeek V4 Flash 0423, the interview's model). If your OpenRouter workspace has guardrails, the model must be allowed there.

Changes to `env/` are built into the binary, so rebuild or redeploy after editing.

## Running it locally

You need Go 1.22 or later.

```bash
FAKE_LLM=1 tools/run-local.sh
```

This uses canned replies from [`tools/fake-openrouter`](tools/fake-openrouter/main.go), so you can click through the whole UI without an API key. To use the real model, set `OPENROUTER_API_KEY` (or put it in `~/.openrouter-env`) and run without `FAKE_LLM`:

```bash
tools/run-local.sh
```

Open http://localhost:8080 and paste the development public key it prints (`~/.ipytutor-dev/auth-public-key.pem`).

Run the tests:

```bash
cd openrouter-app && go test ./...
```

They cover the curriculum files (including the scope check on every lesson plan), command detection and problem parsing. End-to-end tests drive whole student sessions through the HTTP handlers against a fake OpenRouter: jumping between weeks and lessons, each practice format's steps, and recovering after a restart.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `OPENROUTER_API_KEY` | (required) | OpenRouter key |
| `AUTH_PRIVATE_KEY_FILE` or `AUTH_PRIVATE_KEY` | (required) | RSA private key for login |
| `TUTOR_MODEL` | `deepseek/deepseek-v4-flash` (DeepSeek V4 Flash 0423) | OpenRouter model |
| `PORT`, `LISTEN_ADDR` | `8080`, all interfaces | Listen address (the systemd unit uses `127.0.0.1:8090`) |
| `OPENROUTER_URL` | OpenRouter's endpoint | Point at a fake for testing |
| `OPENROUTER_HTTP_REFERER`, `OPENROUTER_APP_TITLE` | none, `iPyTutor` | Attribution headers sent to OpenRouter |

## Deploying

The scripts deploy iPyTutor as its own service, so it can share a server with iPyIntervu without touching it:

| | iPyTutor | iPyIntervu |
|---|---|---|
| systemd service | `ipytutor` | `openrouter-app` |
| Binary | `/usr/local/bin/ipytutor` | `/usr/local/bin/openrouter-app` |
| Secrets | `/etc/ipytutor/` | `/etc/openrouter-app/` |
| Port (localhost) | `8090` | `8080` |

On an Ubuntu 24.04 server with Go installed, with the repository cloned to, for example, `/home/manager/iPyTutorServer`:

```bash
cd /home/manager/iPyTutorServer/openrouter-app
```

**1. Choose the login key.** To let students use the public key they already have for iPyIntervu, use the same private key. Otherwise, make a new pair and give students `env/ipytutor-pub.pem`:

```bash
openssl genpkey -algorithm RSA -out env/ipytutor-key.pem -pkeyopt rsa_keygen_bits:2048
openssl rsa -in env/ipytutor-key.pem -pubout -out env/ipytutor-pub.pem
chmod 600 env/ipytutor-key.pem
```

**2. Install secrets.** This prompts for the OpenRouter key and installs the private key. Pass a path to use another key, such as iPyIntervu's `/etc/openrouter-app/auth-private-key.pem`:

```bash
sudo ./deploy/install-secrets.sh
```

**3. Build, install and start the service:**

```bash
sudo ./deploy/install.sh
curl http://127.0.0.1:8090/healthz
```

**4. Enable HTTPS** (needed for the browser's login encryption). Point a domain or subdomain, such as `tutor.aalang.org`, at the server first:

```bash
sudo ./deploy/setup-https.sh tutor.aalang.org admin@aalang.org
```

**Updating** after `git pull`:

```bash
./deploy/redeploy.sh
```

**Staging:** `./deploy/staging.sh` runs the current source on `127.0.0.1:8091`, next to production.

**Rotating the login key:** `sudo ./deploy/cycle_keys.sh` (try `--dry-run` first). Students must paste the new public key; conversations in progress are lost.

The tutor stores nothing on disk, so there's no student data to back up. Conversations live in the server's memory, so run one server (or use sticky sessions behind a load balancer).

### Deploy scripts

| Script | When to run | Requires root |
|---|---|---|
| `deploy/build.sh` | Compile the Linux binary only | No |
| `deploy/install-secrets.sh` | First-time secret setup | Yes |
| `deploy/install.sh` | First-time app and systemd install | Yes |
| `deploy/setup-https.sh` | First-time nginx and HTTPS setup | Yes |
| `deploy/redeploy.sh` | Code or content updates | Partly (`sudo` to install and restart) |
| `deploy/staging.sh` | Try changes next to production | Partly |
| `deploy/cycle_keys.sh` | Rotate the login keypair | Yes |
| `deploy/startup.sh`, `deploy/shutdown.sh` | Start or stop nginx | Yes |

## Troubleshooting

- **`curl http://127.0.0.1:8090/healthz` fails:** check `sudo systemctl status ipytutor` and `sudo journalctl -u ipytutor -n 50`.
- **The tutor replies "couldn't reach the tutor model":** check the OpenRouter key in `/etc/ipytutor/env` and the `[tutor] llm` log lines for the error.
- **Authentication fails in the browser:** use `https://`, and make sure the public key students paste matches the private key in `/etc/ipytutor/`.
- **`502 Bad Gateway` from nginx:** the app must be running on `127.0.0.1:8090`.
- **Model calls fail with `404 ... guardrail`:** your OpenRouter workspace's guardrails don't allow the model. Allow `TUTOR_MODEL`'s model there, or choose an allowed one.
