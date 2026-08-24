# End-to-end sweep

Seven scripts that drive a running Ptium the way a person and a client would, and
report what broke. They found the empty-upload five hundred, the responsive
stylesheet that never applied, the Escape that threw away a sentence, and the
deck titles that dropped their own subject.

```bash
# any reachable Ptium with development auth on
export PTIUM_URL=http://localhost:8099
export PTIUM_DEV_SECRET=devsecret-devsecret-devsecret-devsecret

python3 scripts/e2e/api.py      # every REST endpoint, the happy paths
python3 scripts/e2e/edges.py    # the limits: 50 slides, 16 MiB, two editors at once
python3 scripts/e2e/ui.py .     # every screen renders, nothing logs an error
python3 scripts/e2e/flows.py .  # generate, edit, apply source, save a slide, export
python3 scripts/e2e/deep.py .   # MCP, a template round trip, the presenter window, keyboard
python3 scripts/e2e/package.py  # what PowerPoint sees, read back with python-pptx

# several people at once, against a server built with the race detector
cd server && go build -race -o /tmp/ptium-race ./cmd/ptium
DATABASE_URL=... /tmp/ptium-race > /tmp/ptium-race.log 2>&1 &
python3 scripts/e2e/race.py --log /tmp/ptium-race.log --seconds 90
```

`race.py` is the one that needs a server built with `-race`: it drives readers,
writers, administrators, image readers and a generation at the same time, then
reads the detector's own output. It found the generator's shared settings and a
first sign-in answering five hundred when a browser opened several tabs of it at
once. It cleans up every deck it queues.

The editor's pure logic — how a measurement is written in the reader's words,
how a slide's fields map to template slots — is tested next to the code instead,
with `npm test` in `web/`. A browser is the wrong place to find out that one
translation rule is wrong.

The browser scripts need Playwright (`pip install playwright && playwright install
chromium`) and `package.py` needs `python-pptx`; the first two need nothing but
Python. Each exits non-zero on a
failure and prints the request, the status and the body — enough to go straight
to the code.

Every script is re-runnable against the same database: names carry a per-run
suffix, so nothing collides with what an earlier run left behind.
