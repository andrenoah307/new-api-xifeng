const ru: Record<string, string> = {
  // Sidebar
  'Risk Control': 'Контроль рисков',
  'Group Monitoring': 'Мониторинг групп',
  'Invitation Codes': 'Коды приглашений',
  Tickets: 'Тикеты',
  'Top-up History': 'История пополнений',
  'Ticket Admin': 'Управление тикетами',

  // Risk Control
  'Distribution Detection': 'Обнаружение распределения',
  'Content Moderation': 'Модерация контента',
  Enforcement: 'Применение',

  // Monitoring
  Online: 'Онлайн',
  Offline: 'Офлайн',
  'Next auto refresh {{c}}': 'Следующее автообновление: {{c}}',
  'Refresh successful': 'Обновление выполнено',
  'Configure in System Settings - Group Monitoring':
    'Настройте в разделе «Системные настройки — Мониторинг групп»',
  'No groups matching "{{kw}}"': 'Нет групп, соответствующих «{{kw}}»',
  'CNY/USD': 'CNY/USD',
  'No history data': 'Нет исторических данных',
  'Group Detail': 'Сведения о группе',
  'Status Timeline': 'Хронология состояний',
  'Channel Details': 'Сведения о каналах',
  'No channel data': 'Нет данных о каналах',
  'Cache Rate': 'Коэффициент кэширования',
  'Last Test': 'Последняя проверка',
  'History (latest)': 'История (последние данные)',
  'History ({{n}} entries)': 'История ({{n}} записей)',
  'Left to right: old to new': 'Слева направо: от старых к новым',

  // Tickets
  'Create Ticket': 'Создать тикет',
  'Close Ticket': 'Закрыть тикет',
  'Ticket Detail': 'Детали тикета',
  'Not Invoiced': 'Не выставлен счёт',
  'Invoice Status': 'Статус счёта',
  Open: 'Открыт',
  Processing: 'В обработке',
  Resolved: 'Решён',
  Closed: 'Закрыт',

  // Invitation Codes
  'Create Invitation Code': 'Создать код приглашения',
  Active: 'Активен',
  Disabled: 'Отключён',
  Expired: 'Истёк',

  // Top-up
  Success: 'Успешно',
  Pending: 'Ожидание',
  Failed: 'Ошибка',

  // Channel Extensions
  'Custom Extensions': 'Пользовательские расширения',
  'Pressure Cooling': 'Контроль давления',
  'Channel Rate Limit': 'Ограничение канала',
  'Error Filter Rules': 'Правила фильтрации ошибок',
  'Risk Control Headers': 'Заголовки контроля рисков',
  'Pressure cooling, rate limiting, error filtering and risk-control headers for this channel.':
    'Охлаждение при нагрузке, ограничение частоты, фильтрация ошибок и заголовки контроля рисков для этого канала.',

  // System Settings
  'Email Templates': 'Шаблоны email',
  'Ticket Settings': 'Настройки тикетов',
  'Group Monitoring Settings': 'Настройки мониторинга',
  'Config saved': 'Настройки сохранены',
  'Operation successful': 'Операция успешна',
  'Operation failed': 'Ошибка операции',
  Save: 'Сохранить',
  'Saving...': 'Сохранение...',

  // Wallet and administration
  'Commission Rate': 'Ставка комиссии',
  'Minimum Transfer Amount': 'Минимальная сумма перевода',
  'Minimum amount (in currency units) required for affiliate reward transfers. Set to 0 for no minimum.':
    'Минимальная сумма (в денежных единицах) для перевода партнёрских вознаграждений. Укажите 0, чтобы убрать минимум.',
  'Needs manual handling (reject / offline red invoice)':
    'Требуется ручная обработка (отклонение / офлайн-сторно счёта)',
  'Commission from this user': 'Комиссия от этого пользователя',
  'Total commission': 'Общая комиссия',
  'Clawed back': 'Удержано',
  'Clawback available': 'Доступно для удержания',
  'Inviter aff balance': 'Партнёрский баланс пригласившего',
  'Show all ({{count}})': 'Показать все ({{count}})',
  'Claw back inviter commission': 'Удержать комиссию пригласившего',
  'Clawback amount': 'Сумма удержания',
  'Amount exceeding the clawback limit will be capped automatically. A negative commission record will be visible to the inviter.':
    'Сумма свыше лимита удержания будет автоматически ограничена. Пригласивший увидит отрицательную запись о комиссии.',
  'Refund Clawback': 'Удержание при возврате',
  'Add Entry': 'Добавить запись',
  'Invalid JSON — fix it here before switching to visual mode':
    'Недопустимый JSON — исправьте его здесь перед переходом в визуальный режим',
  'Inviter user ID. Set to 0 to clear the inviter.':
    'ID пользователя, который пригласил. Укажите 0, чтобы удалить пригласившего.',

  // Auto Group
  'Auto Group': 'Автогруппы',
}

export default ru
