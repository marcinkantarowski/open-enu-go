/**
 * llms.txt (llmstxt.org): the site described for a language model - its name,
 * one paragraph, and every page with what it is about. Built from the same
 * titles and descriptions a search engine reads, so writing those well is the
 * whole of the work.
 */
const OTHER_LANGUAGES = { en: 'English', pl: 'Polski' }

export default defineEventHandler(async (event) => {
  const { name, defaultLocale } = siteConfig(event)
  const pages = await sitePages(event)
  setHeader(event, 'Content-Type', 'text/plain; charset=utf-8')

  const list = (code: string) => pages
    .map(page => page.versions.find(v => v.code === code))
    .filter(v => v !== undefined)
    .map(v => `- [${v.title}](${v.url}): ${v.description}`)

  const home = pages.find(p => p.path === '/')?.versions.find(v => v.code === defaultLocale)

  return [
    `# ${name}`,
    '',
    ...(home ? [`> ${home.description}`, ''] : []),
    '## Pages',
    '',
    ...list(defaultLocale),
    '',
    // The other language under its own name, as a reader of it would look for it.
    ...Object.entries(OTHER_LANGUAGES)
      .filter(([code]) => code !== defaultLocale)
      .flatMap(([code, heading]) => [`## ${heading}`, '', ...list(code), '']),
  ].join('\n')
})
