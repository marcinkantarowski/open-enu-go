# Route middleware

| File | Kind | What it does |
|---|---|---|
| `auth.global.ts` | global | Requires a session unless the page sets `definePageMeta({ public: true })`. |
| `guest.ts` | named | Sends an already-signed-in visitor away from login/register/reset. |
| `permission.ts` | named | Refuses a route whose `meta.permission` the session does not hold. |

Authentication is global and permissions are opt-in, and the asymmetry is
deliberate: forgetting the global one is impossible, and forgetting the named one
produces a page full of 403s rather than a leak - the server checks every request
regardless.
