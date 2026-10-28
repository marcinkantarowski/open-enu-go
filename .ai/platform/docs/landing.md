# The landing site

`landing/` ships as a **template, not as a site about this platform**. Out of the box it is
one "coming soon" page with two buttons - create an account, sign in - plus contact, privacy
and terms. A project is expected to replace the words and switch sections on as it has
something to say. Nothing here needs a new route or a new component.

## Where the words are

| What | Where |
|---|---|
| A page | `landing/content/en/<page>.md` **and** `landing/content/pl/<page>.md` - always both; `make i18n-check` fails on a page that exists in one locale only |
| Header, footer, language names | `landing/i18n/locales/{en,pl}.json` |
| The product's name | `PROJECT_NAME` in `.env` (set by `make init`), which `make env` writes into `landing/.env` as `NUXT_PUBLIC_APP_NAME` - never typed into a template |
| Links | `/contact` stays on the site, in the reader's language; `app:/register` goes to the app host, which derives from `DOMAIN` (`SiteLink.vue`) |
| The share image, the icon | `landing/public/og.png` (1200x630) and `landing/public/favicon.svg` - plain placeholders; replace both |

The home page is `index.md`. Every other file is served by `app/pages/[...slug].vue` at the
path its filename gives it: `content/en/about.md` is `/about`, and `content/pl/about.md` is
`/pl/about` (`strategy: 'prefix_except_default'`, the default being `DEFAULT_LOCALE`).
**Each language has its own URL** - unlike the two SPAs - because a crawler sends no cookie
and no `Accept-Language`, so a language that shares a URL is never indexed. For the same reason
the site never redirects by the browser's language: `/` is always `DEFAULT_LOCALE`, and
`make smoke` asks for it with the other language preferred. To put a page in
the header or footer, add it to `nav` in `SiteHeader.vue` / `columns` in `SiteFooter.vue`,
with a key in both catalogues.

## Sections

A section appears when its frontmatter key is present and is absent otherwise: `hero` on
the home page, `plans` on any other page, the rest on any page. The schema is
`landing/content.config.ts`; a typo in a key fails the build rather than silently rendering
nothing. The markdown body below the frontmatter is rendered as prose, and may be empty.

### `hero` - home page only

```yaml
hero:
  eyebrow: Coming soon            # optional
  title: Something good is being built here
  subtitle: One or two sentences.
  primary:   { label: Create an account, to: app:/register }
  secondary: { label: Sign in, to: app:/login }   # optional
  note: Free to start                              # optional
```

### `features`, `steps`

```yaml
features:
  eyebrow: What you get           # optional
  title: Three reasons to try it
  lead: One sentence under the title.   # optional
  items:
    - { icon: "⚡", title: Fast, body: One or two sentences. }
steps:
  title: How it works
  items:
    - { title: Sign up, body: One or two sentences. }
```

### `plans` - turns a page into a pricing page

Create `content/{en,pl}/pricing.md` and link it from the header.

```yaml
title: Pricing
description: Simple plans that grow with your team.
plans:
  - name: Starter
    price: Free
    description: For trying things out.
    features: [One workspace, Up to 3 members]
    cta: { label: Start for free, to: app:/register }
  - name: Team
    price: "€49"
    period: per workspace / month   # optional
    highlighted: true               # optional - shows the "most popular" badge
    description: For teams running their business on it.
    features: [Unlimited members, Email support]
    cta: { label: Start a trial, to: app:/register }
```

### `faqs`, `cta`

```yaml
faqs:
  title: Questions
  items:
    - { q: Can I change plans later?, a: Yes. }
cta:
  title: Ready when you are
  body: One sentence.
  primary: { label: Get started, to: app:/register }
```

## Found by search engines and by LLMs

All of it is generated from the content files. There is nothing to maintain by hand, and
adding a page adds it everywhere.

| What | Where it comes from |
|---|---|
| `<title>`, description, canonical, `hreflang`, Open Graph, Twitter card, robots meta, JSON-LD | `app/composables/usePageSeo.ts` - called by both page templates |
| `/robots.txt` | `server/routes/robots.txt.ts` |
| `/sitemap.xml` - every page, every language, with `hreflang` alternates and `x-default` | `server/routes/sitemap.xml.ts` |
| `/llms.txt` - the site described for a language model ([llmstxt.org](https://llmstxt.org)) | `server/routes/llms.txt.ts` |
| The list of pages all three are built from | `server/utils/site.ts` |
| `X-Robots-Tag` on every response off production | `server/middleware/noindex.ts` |

Three frontmatter keys, all optional, on any page:

```yaml
noindex: true          # out of search results, the sitemap and llms.txt
image: /og-pricing.png # this page's share image; a file in landing/public/
updated: 2026-09-21    # becomes <lastmod> in the sitemap
```

**Only production is indexable.** `make env` writes `NUXT_PUBLIC_INDEXABLE=true` when
`APP_STAGE=production` and `false` everywhere else - and the code defaults to `false`, so a
missing variable hides a site instead of publishing a staging server. When it is `false`,
`robots.txt` says `Disallow: /`, every response carries `X-Robots-Tag: noindex, nofollow`,
and every page carries the meta tag. Seeing that locally is correct, not a bug.

JSON-LD is `WebSite` + `Organization` on the home page and `WebPage` elsewhere; a page with
`faqs:` also gets `FAQPage`, which is what makes the questions eligible to show in results.

To stay in search but refuse AI training crawlers, add groups to `robots.txt.ts`:
`User-agent: GPTBot`, `ClaudeBot`, `Google-Extended`, `PerplexityBot`, `CCBot`, each with
`Disallow: /`. The default allows them: a marketing site usually wants to be quoted.

## Rules for writing a page

They are the part no template can do for you. Two are checked - a page in one language only
(`make i18n-check`) and an internal link to a page that does not exist (`make docs-check`);
the rest are on whoever writes the page.

| Rule | Why |
|---|---|
| `title`: present, at most 60 characters, unique within the language | It is the link in the results. Longer is cut off; the site name is appended for you |
| `description`: 50 to 160 characters, unique within the language | It is the snippet under the link, the share text, and the page's entry in `llms.txt`. Say what the page is for, in a sentence a person would click |
| No `# ` heading in the body; start at `## ` and do not skip levels | The template renders `title` as the page's only `<h1>`. Two of them, or a jump to `###`, breaks the outline crawlers and screen readers read |
| Every image has alt text - the words between the square brackets of `![a dashboard with three charts]` | An image with none is invisible to search, to an LLM and to a screen reader |
| Filenames are lowercase words joined by hyphens: `how-it-works.md` | The filename is the URL. Keep it short, in words a person would search for, and the same in every language |
| Every page exists in every language | A missing translation would break the `hreflang` pair the sitemap promises |
| Internal links are written `/contact`, never with a host or a `/pl` prefix | The reader's language is added for you; a hard-coded prefix sends Polish readers to English pages |

Beyond the table: lead with the words a customer would type, put the point of the page in
its first paragraph, one topic per page, and link related pages to each other with text
that says where the link goes - never "click here".

## What must stay true

- The home page carries `<meta name="app-role" content="marketing">` (`nuxt.config.ts`).
  `make smoke` looks for it, so that a crashed app answering 200 is not mistaken for the site.
- The privacy policy and the terms are starting templates and say so at the top. Have them
  reviewed before publishing; do not delete the notice until that has happened.
- The landing site renders on the server and must stay up when the API is down: it makes no
  API calls. Server rendering is also what makes it indexable - do not move content behind
  client-only rendering.
- A new page template must call `usePageSeo(page)`, or its pages have no canonical URL, no
  `hreflang` and no structured data. There are two templates so that this stays checkable.
