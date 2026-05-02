import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import type { TicketMessage } from '../api'
import { TicketMessageItem } from './ticket-message-item'

interface TicketConversationProps {
  messages: TicketMessage[]
  currentUserId: number
  loading?: boolean
}

export function TicketConversation({
  messages,
  currentUserId,
  loading,
}: TicketConversationProps) {
  const { t } = useTranslation()
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
  }, [messages.length])

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <span className="text-muted-foreground text-sm">{t('Loading...')}</span>
      </div>
    )
  }

  if (messages.length === 0) {
    return (
      <div className="text-muted-foreground py-12 text-center text-sm">
        {t('No messages yet')}
      </div>
    )
  }

  return (
    <div className="space-y-4 py-4">
      {messages.map((msg) => (
        <TicketMessageItem
          key={msg.id}
          message={msg}
          isMine={Number(msg.user_id) === Number(currentUserId)}
        />
      ))}
      <div ref={bottomRef} />
    </div>
  )
}
