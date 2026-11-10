export interface CustomNavigationLink {
  id: string
  title: string
  url: string
  mode: 'external' | 'iframe'
  header: boolean
  sidebar: boolean
}

export function parseCustomNavigationLinks(
  raw: unknown
): CustomNavigationLink[] {
  try {
    const links: unknown = typeof raw === 'string' ? JSON.parse(raw) : raw
    if (!Array.isArray(links)) return []
    return links.filter((link): link is CustomNavigationLink => {
      if (
        !link ||
        typeof link.id !== 'string' ||
        !/^[a-zA-Z0-9_-]{1,64}$/.test(link.id) ||
        typeof link.title !== 'string' ||
        !link.title.trim()
      ) {
        return false
      }
      try {
        const url = new URL(link.url)
        return (
          !url.username &&
          !url.password &&
          (link.mode === 'external'
            ? ['http:', 'https:'].includes(url.protocol)
            : link.mode === 'iframe' && url.protocol === 'https:')
        )
      } catch {
        return false
      }
    })
  } catch {
    return []
  }
}

export function customNavigationHref(link: CustomNavigationLink): string {
  return link.mode === 'iframe' ? `/links/${link.id}` : link.url
}
