const zhTW: Record<string, string> = {
  // Monitoring
  'Next auto refresh {{c}}': '下次自動重新整理 {{c}}',
  'Refresh successful': '重新整理成功',
  'Configure in System Settings - Group Monitoring':
    '請在 系統設定 - 分組監控 中設定',
  'No groups matching "{{kw}}"': '沒有符合 "{{kw}}" 的分組',
  'CNY/USD': '人民幣/美元',
  'No history data': '暫無歷史資料',
  'Group Detail': '分組詳情',
  'Status Timeline': '狀態時間線',
  'Channel Details': '渠道明細',
  'No channel data': '暫無渠道資料',
  'Cache Rate': '快取率',
  'Last Test': '最近測試',
  'History (latest)': '歷史（最新）',
  'History ({{n}} entries)': '歷史（{{n}} 條）',
  'Left to right: old to new': '從左到右：由舊到新',

  // Tickets
  'Invoice Ticket': '發票工單',
  'Apply for Invoice': '申請開票',
  Date: '日期',
  'Not Invoiced': '未開票',
  'Invoice Status': '開票狀態',
  'Pending Issuance': '待開具',
  Issued: '已開具',

  // Channel extensions
  'Pressure cooling, rate limiting, error filtering and risk-control headers for this channel.':
    '本渠道的壓力冷卻、渠道限流、上游錯誤過濾與風控識別標頭設定。',

  // Wallet and administration
  'Commission Rate': '返傭比例',
  'Minimum Transfer Amount': '最低轉出金額',
  'Minimum amount (in currency units) required for affiliate reward transfers. Set to 0 for no minimum.':
    '推薦獎勵轉出到餘額的最低金額（按貨幣單位）。設為 0 則不限制。',
  'Needs manual handling (reject / offline red invoice)':
    '需人工處理（駁回/線下紅沖）',
  'Commission from this user': '該使用者產生的返傭',
  'Top-Up Amount': '儲值金額',
  Commission: '返傭',
  'Total commission': '返傭合計',
  'Clawed back': '已扣回',
  'Clawback available': '可扣回',
  'Inviter aff balance': '邀請人返傭餘額',
  'Show all ({{count}})': '展開全部（{{count}} 條）',
  'Claw back inviter commission': '扣回邀請人返傭',
  'Clawback amount': '扣回金額',
  'Amount exceeding the clawback limit will be capped automatically. A negative commission record will be visible to the inviter.':
    '超出可扣回上限的金額會被自動封頂；扣回會生成一條負數返傭記錄，邀請人可在錢包中看到。',
  'Refund Clawback': '退款扣回',
  'Add Entry': '新增條目',
  'Invalid JSON — fix it here before switching to visual mode':
    'JSON 無效——請先在此修復後再切換到可視化模式',
  'Inviter user ID. Set to 0 to clear the inviter.':
    '邀請人的使用者 ID。填 0 表示清除邀請人。',
}

export default zhTW
