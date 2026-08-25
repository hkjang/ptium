# End-to-end sweep

Nine scripts that drive a running Ptium the way a person and a client would, and
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
python3 scripts/e2e/tenancy.py  # whose is whose: accounts, keys, admin doors, links
python3 scripts/e2e/withmodel.py # generation with the provider, not the offline writer

# the released image itself, on a database that has never been used
python3 scripts/e2e/firstrun.py

# and on one an old release wrote, which is what an upgrade runs
python3 scripts/e2e/upgrade.py

# several people at once, against a server built with the race detector
cd server && go build -race -o /tmp/ptium-race ./cmd/ptium
DATABASE_URL=... /tmp/ptium-race > /tmp/ptium-race.log 2>&1 &
python3 scripts/e2e/race.py --log /tmp/ptium-race.log --seconds 90
```

`tenancy.py` needs nothing but development auth, and asks the questions that are
only visible from another chair: what one account can reach of another's, what a
key may do beyond its scopes, which doors want the administrator role, and what a
link hands to whoever holds it. It found nothing the day it was written — it was
written the day after a link was found drawing a slide marked `!skip`.

It also asks what a person can make the product draw. The workspace and the
shared page render these SVGs as HTML, so anything typed into a deck that
survives as markup is script running in whoever opens it. Every drawing the
product hands to a browser is fed `</text><script>…` and read back, and every
address drawn as a link has to be one that can sit inside an attribute and a
scheme a reader may follow. Taking the escaping out of `escapeText` makes four
of those checks fail.

`withmodel.py` is the only one that asks the model. Every other sweep runs
offline, so the writer a deployment actually uses — the provider, the repair
passes, the notes a deck keeps about what it did — was checked by nothing. It
switches `ai.provider` on, generates in two languages, and puts the setting back
the way it found it whatever happens. It skips itself when no provider is
configured. What it holds: the deck completes, has no defects, is written in the
language it was asked for, and says which numbers it introduced that the brief
never gave. Names are not the deck's words — an image is named by whoever
uploaded it and a layout by whoever made the template — so those lines are not
read for language.

`race.py` is the one that needs a server built with `-race`: it drives readers,
writers, administrators, image readers and a generation at the same time, then
reads the detector's own output. It found the generator's shared settings and a
first sign-in answering five hundred when a browser opened several tabs of it at
once. It cleans up every deck it queues.

`firstrun.py` is the only one that does not talk to the development server. It
loads the image built for the release, starts it against an empty PostgreSQL,
signs in as the bootstrap administrator and makes a deck — the first hour of a
deployment, which every other check here skips because it runs against a binary
from the working tree and a database that has been alive for days. It needs
docker and a postgres image on the host.

`upgrade.py` is its pair: it lets an old image write the database, brings the
new one up on it, and asks whether a deck written that long ago still opens,
measures, draws and exports. It skips itself when no older image is on the host.

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
