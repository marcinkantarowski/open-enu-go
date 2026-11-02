# The landing found by search engines and LLMs, the apps never

Status: done (2026-11-02)

## Why

Brought back from a project (cyberenu), where a review of the landing for search and for AI
assistants found the gaps below in what the platform ships:

- the app and the console carried no `noindex` and answered `/robots.txt` with the page;
- `www.` and a trailing slash were a second address for every page;
- the JSON-LD said nothing of the company behind the product;
- `llms.txt` printed an empty heading for a language the site does not offer;
- a page whose heading is too long for a search result had no shorter title;
- a third-party script on the landing (a chat widget) was refused by the CSP;
- the icon was the landing's alone.

## What

- **Never indexed**: `ui-kit/server/middleware/noindex.ts` runs in all three apps. An app
  without `runtimeConfig.public.indexable === true` sends `X-Robots-Tag: noindex, nofollow`
  on every response and answers `/robots.txt` with `Disallow: /`. Only the landing declares
  it, and only on production; the landing's own `noindex.ts` is gone.
- **One address**: `landing/server/middleware/canonical.ts` sends `www.` and a path with a
  trailing slash, 301, to the host of `siteUrl`, query kept.
- **llms.txt**: headings in the default language; articles (pages with `published:`) apart
  from the pages; another language only when it is in `LOCALES`.
- **JSON-LD** (`usePageSeo`): an `@id` for the organization and the site; `SoftwareApplication`
  on the home page; `Article` for a page with `published:`; `BreadcrumbList` on every other
  page; `organization` in the landing's `app.config.ts`, when set, as the parent company.
  Open Graph gains the site name, locale, alternates and the large Twitter card.
- **Frontmatter**: `headTitle`, `published`, `tags`.
- **usePageSeo(page, { parent, image })**: a project's template passes what the frontmatter
  does not know - the page an article sits under, a share image of its own.
- **CSP**: `csp: { script, connect, img }` in an app's `app.config.ts` adds origins to those
  three directives, each reduced to an origin.
- **The icon**: `ui-kit/public/favicon.svg` and `apple-touch-icon.png`, linked from ui-kit's
  `nuxt.config.ts`, so all three apps show the same; the PNG is the JSON-LD logo.
- **Markdown tables** scroll sideways on a phone (`prose.css`).

## Decisions

- **The middleware lives in ui-kit, not in each app.** An app that forgets it is indexed;
  one that must be indexed says so. The default is the safe one.
- **The redirect, not only the canonical link.** The link asks a search engine to merge two
  addresses; the 301 stops the second one existing.
- **The company is configuration, not content.** It is the same on every page and in every
  language, and a project without one gets the product alone.

## Tests

`make smoke`: `www.` and `/contact/` answer 301 to the canonical address; the app's and the
console's `robots.txt` say `Disallow: /`; `llms.txt` lists the home page. `make typecheck`
for the three apps.

## Changelog

- 2026-11-02 - first version, from cyberenu.
