const ja: Record<string, string> = {
  // Sidebar
  'Risk Control': 'リスク管理',
  'Group Monitoring': 'グループ監視',
  'Invitation Codes': '招待コード',
  Tickets: 'チケット',
  'Top-up History': 'チャージ履歴',
  'Ticket Admin': 'チケット管理',

  // Risk Control
  'Distribution Detection': '配信検出',
  'Content Moderation': 'コンテンツモデレーション',
  Enforcement: '執行',

  // Monitoring
  Online: 'オンライン',
  Offline: 'オフライン',
  'Next auto refresh {{c}}': '次回の自動更新 {{c}}',
  'Refresh successful': '更新しました',
  'Configure in System Settings - Group Monitoring':
    'システム設定 - グループ監視で設定してください',
  'No groups matching "{{kw}}"': '「{{kw}}」に一致するグループはありません',
  'CNY/USD': '人民元/米ドル',
  'No history data': '履歴データがありません',
  'Group Detail': 'グループ詳細',
  'Status Timeline': 'ステータス履歴',
  'Channel Details': 'チャンネル詳細',
  'No channel data': 'チャンネルデータがありません',
  'Cache Rate': 'キャッシュ率',
  'Last Test': '最終テスト',
  'History (latest)': '履歴（最新）',
  'History ({{n}} entries)': '履歴（{{n}}件）',
  'Left to right: old to new': '左から右：古い順',

  // Tickets
  'Create Ticket': 'チケット作成',
  'Close Ticket': 'チケットを閉じる',
  'Ticket Detail': 'チケット詳細',
  'Not Invoiced': '未請求',
  'Invoice Status': '請求書ステータス',
  Open: 'オープン',
  Processing: '処理中',
  Resolved: '解決済み',
  Closed: 'クローズ',

  // Invitation Codes
  'Create Invitation Code': '招待コード作成',
  Active: '有効',
  Disabled: '無効',
  Expired: '期限切れ',

  // Top-up
  Success: '成功',
  Pending: '保留中',
  Failed: '失敗',

  // Channel Extensions
  'Custom Extensions': 'カスタム拡張',
  'Pressure Cooling': '圧力冷却',
  'Channel Rate Limit': 'チャネルレート制限',
  'Error Filter Rules': 'エラーフィルタルール',
  'Risk Control Headers': 'リスク管理ヘッダー',
  'Pressure cooling, rate limiting, error filtering and risk-control headers for this channel.':
    'このチャンネルの負荷冷却、レート制限、エラーフィルタリング、リスク制御ヘッダーを設定します。',

  // System Settings
  'Email Templates': 'メールテンプレート',
  'Ticket Settings': 'チケット設定',
  'Group Monitoring Settings': 'グループ監視設定',
  'Config saved': '設定を保存しました',
  'Operation successful': '操作成功',
  'Operation failed': '操作失敗',
  Save: '保存',
  'Saving...': '保存中...',

  // Wallet and administration
  'Commission Rate': 'コミッション率',
  'Minimum Transfer Amount': '最低振替額',
  'Minimum amount (in currency units) required for affiliate reward transfers. Set to 0 for no minimum.':
    'アフィリエイト報酬を振り替えるために必要な最低額（通貨単位）。制限なしの場合は0に設定します。',
  'Needs manual handling (reject / offline red invoice)':
    '手動対応が必要（却下 / オフラインでの赤伝票処理）',
  'Commission from this user': 'このユーザーからのコミッション',
  'Total commission': 'コミッション合計',
  'Clawed back': '回収済み',
  'Clawback available': '回収可能額',
  'Inviter aff balance': '招待者のアフィリエイト残高',
  'Show all ({{count}})': 'すべて表示（{{count}}件）',
  'Claw back inviter commission': '招待者のコミッションを回収',
  'Clawback amount': '回収額',
  'Amount exceeding the clawback limit will be capped automatically. A negative commission record will be visible to the inviter.':
    '回収上限を超える金額は自動的に上限へ調整されます。負のコミッション記録が招待者に表示されます。',
  'Refund Clawback': '返金回収',
  'Add Entry': 'エントリを追加',
  'Invalid JSON — fix it here before switching to visual mode':
    'JSONが無効です — ビジュアルモードに切り替える前にここで修正してください',
  'Inviter user ID. Set to 0 to clear the inviter.':
    '招待者のユーザーID。招待者を解除するには0を設定します。',

  // Auto Group
  'Auto Group': '自動グループ',
}

export default ja
