import { defineCollection, defineContentConfig, z } from '@nuxt/content'

/**
 * One collection per locale, with the same schema.
 *
 * The locale directory is stripped from the path (`prefix: ''`), so
 * `content/pl/contact.md` answers `/contact` exactly as its English twin does:
 * the URL names the page, and the visitor's locale picks the file. That only
 * works while both directories hold the same set of files, which is why
 * `make i18n-check` compares them.
 *
 * Marketing copy is frontmatter plus a markdown body. The page templates own
 * layout, never words. Every section below is optional and works on every
 * page - the shipped home page uses `hero` alone. Shapes, with examples:
 * .ai/platform/docs/landing.md
 */
const link = z.object({
  label: z.string(),
  // A site path (`/contact`), or `app:/register` for a page on the app host.
  to: z.string(),
})

const section = <T extends z.ZodTypeAny>(item: T) => z.object({
  eyebrow: z.string().optional(),
  title: z.string(),
  lead: z.string().optional(),
  items: z.array(item),
})

const schema = z.object({
  // Rules for both, and for everything else a crawler reads: `make seo-check`.
  title: z.string(),
  description: z.string(),
  // Leave a page out of search results, the sitemap and llms.txt.
  noindex: z.boolean().optional(),
  // The social-share image, as a site path. Defaults to /og.png.
  image: z.string().optional(),
  // ISO date of the last meaningful change; becomes <lastmod> in the sitemap.
  updated: z.string().optional(),
  hero: z.object({
    eyebrow: z.string().optional(),
    title: z.string(),
    subtitle: z.string(),
    primary: link,
    secondary: link.optional(),
    note: z.string().optional(),
  }).optional(),
  features: section(z.object({ icon: z.string(), title: z.string(), body: z.string() })).optional(),
  steps: section(z.object({ title: z.string(), body: z.string() })).optional(),
  plans: z.array(z.object({
    name: z.string(),
    price: z.string(),
    period: z.string().optional(),
    description: z.string(),
    features: z.array(z.string()),
    cta: link,
    highlighted: z.boolean().optional(),
  })).optional(),
  faqs: section(z.object({ q: z.string(), a: z.string() })).optional(),
  cta: z.object({
    title: z.string(),
    body: z.string(),
    primary: link,
  }).optional(),
})

export default defineContentConfig({
  collections: {
    pages_en: defineCollection({ type: 'page', source: { include: 'en/**/*.md', prefix: '' }, schema }),
    pages_pl: defineCollection({ type: 'page', source: { include: 'pl/**/*.md', prefix: '' }, schema }),
  },
})
