/**
 * llms.txt (llmstxt.org): the site described for a language model - its name,
 * one paragraph, and every page with what it is about. Built from the same
 * titles and descriptions a search engine reads, so writing those well is the
 * whole of the work.
 *
 * Written in the site's default language, articles apart from the pages, and each
 * other language offered (LOCALES) under its own name - none that is not offered.
 */
const HEADINGS: Record<string, { pages: string, articles: string, language: string }> = {
  en: { pages: 'Pages', articles: 'Articles', language: 'English' },
  pl: { pages: 'Strony', articles: 'Artykuły', language: 'Polski' },
}

export default defineEventHandler(async (event) => {
  const { name, defaultLocale, locales } = siteConfig(event)
  const pages = await sitePages(event)
  setHeader(event, 'Content-Type', 'text/plain; charset=utf-8')

  const list = (code: string, articles: boolean) => pages
    .filter(page => Boolean(page.published) === articles)
    .map(page => page.versions.find(v => v.code === code))
    .filter(v => v !== undefined)
    .map(v => `- [${v.title}](${v.url}): ${v.description}`)

  const section = (heading: string, lines: string[]) => (lines.length ? [`## ${heading}`, '', ...lines, ''] : [])
  const words = HEADINGS[defaultLocale] ?? HEADINGS.en!
  const home = pages.find(p => p.path === '/')?.versions.find(v => v.code === defaultLocale)

  return [
    `# ${name}`,
    '',
    ...(home ? [`> ${home.description}`, ''] : []),
    ...section(words.pages, list(defaultLocale, false)),
    ...section(words.articles, list(defaultLocale, true)),
    // The other language under its own name, as a reader of it would look for it.
    ...locales
      .filter(code => code !== defaultLocale && HEADINGS[code])
      .flatMap(code => section(HEADINGS[code]!.language, [...list(code, false), ...list(code, true)])),
  ].join('\n')
})
