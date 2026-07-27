import { api } from '@/lib/api'

/**
 * 公告 AI 翻译：按模型厂商分流协议——
 * Claude 系走 /pg/messages（v1/messages），GPT 系走 /pg/responses（v1/responses），
 * 其余回退 /pg/chat/completions。三者同属 UserAuth+Distribute 计费链（登录管理员账号）。
 */

function isClaudeModel(model: string): boolean {
  return /^claude/i.test(model)
}

function isResponsesModel(model: string): boolean {
  // OpenAI responses 协议模型：gpt / chatgpt / o1|o3|o4 系 / codex
  return /^(gpt|chatgpt|o1|o3|o4|codex)/i.test(model)
}

type PgOptions = {
  model: string
  group: string
  prompt: string
  signal?: AbortSignal
}

function extractClaudeText(data: unknown): string {
  const content = (data as { content?: Array<{ type?: string; text?: string }> })
    ?.content
  if (!Array.isArray(content)) return ''
  return content
    .filter((b) => b?.type === 'text' && typeof b.text === 'string')
    .map((b) => b.text)
    .join('')
    .trim()
}

function extractResponsesText(data: unknown): string {
  const d = data as {
    output_text?: string
    output?: Array<{ content?: Array<{ type?: string; text?: string }> }>
  }
  if (typeof d?.output_text === 'string' && d.output_text.trim()) {
    return d.output_text.trim()
  }
  const out = d?.output
  if (!Array.isArray(out)) return ''
  const parts: string[] = []
  for (const item of out) {
    for (const c of item?.content ?? []) {
      if (c?.type === 'output_text' && typeof c.text === 'string') {
        parts.push(c.text)
      }
    }
  }
  return parts.join('').trim()
}

function extractChatText(data: unknown): string {
  const text = (
    data as { choices?: Array<{ message?: { content?: string } }> }
  )?.choices?.[0]?.message?.content
  return typeof text === 'string' ? text.trim() : ''
}

/**
 * 调用一次模型完成翻译，返回纯文本译文。失败抛错由调用方处理。
 */
export async function translateOnce({
  model,
  group,
  prompt,
  signal,
}: PgOptions): Promise<string> {
  const common = { skipErrorHandler: true, signal } as Record<string, unknown>

  if (isClaudeModel(model)) {
    const res = await api.post(
      '/pg/messages',
      {
        model,
        group,
        max_tokens: 8192,
        stream: false,
        messages: [{ role: 'user', content: prompt }],
      },
      common
    )
    return extractClaudeText(res.data)
  }

  if (isResponsesModel(model)) {
    const res = await api.post(
      '/pg/responses',
      { model, group, stream: false, input: prompt },
      common
    )
    return extractResponsesText(res.data)
  }

  const res = await api.post(
    '/pg/chat/completions',
    {
      model,
      group,
      stream: false,
      messages: [{ role: 'user', content: prompt }],
    },
    common
  )
  return extractChatText(res.data)
}

export function buildTranslatePrompt(
  text: string,
  targetLanguageLabel: string
): string {
  return (
    `You are a professional translator. Translate the following announcement text into ${targetLanguageLabel}. ` +
    `Preserve any Markdown or HTML formatting. Output only the translation, with no explanations or quotes.\n\n` +
    text
  )
}
