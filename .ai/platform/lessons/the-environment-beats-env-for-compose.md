# The environment beats .env for compose

**The trap.** `make deployprod HOST=x DOMAIN=example.com` runs `make ci` on the way. A variable
typed on make's command line reaches every child, and `docker compose --env-file .env` prefers
the environment. The dev stack came up with the production URLs and the browser tests ran
against the live server: logins were refused, and one test registered a real workspace.

**The rule.** A step that must run locally does not inherit what a deploy was typed with, and
the browser tests run only on the DOMAIN in `.env`.

**Where it is held.** `scripts/remote/deploy.sh` clears those variables around `make ci`;
`scripts/dev/e2e.sh` refuses a different DOMAIN before touching anything; `make selftest` proves it.
