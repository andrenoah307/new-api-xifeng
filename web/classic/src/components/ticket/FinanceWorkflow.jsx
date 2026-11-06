import React, { useContext, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button as SemiButton } from '@douyinfe/semi-ui';
import { UserContext } from '../../context/User';
import { API as api } from '../../helpers';
function Button({
  variant,
  ...props
}) {
  return <SemiButton theme={variant === 'outline' ? 'light' : 'solid'} {...props} />;
}
import { createFinanceMoney } from '../../../../shared/finance-money';
const inputClass = 'w-full rounded border bg-transparent p-2 text-sm';
const cashMoney = createFinanceMoney('100', '1');
export function RefundPayoutPanel({
  ticketId,
  onUpdated
}) {
  const {
    t
  } = useTranslation();
  const [userState] = useContext(UserContext);
  const user = userState.user;
  const query = useFinancialQuery(`/api/ticket/refund/${ticketId}/payout`);
  const [key, setKey] = useState(() => crypto.randomUUID());
  const [reference, setReference] = useState('');
  const [evidence, setEvidence] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [clawback, setClawback] = useState(true);
  async function send(status) {
    if (busy) return;
    if (status === 'succeeded' && !window.confirm(t('Confirm paid refund') + ` ¥${((query.data?.cash_cents ?? 0) / 100).toFixed(2)}`)) {
      return;
    }
    if (status === 'failed' && !window.confirm(t('Confirm no payment occurred'))) {
      return;
    }
    setBusy(true);
    setError('');
    try {
      const response = status === 'start' ? await api.post(`/api/ticket/refund/${ticketId}/payout`, {
        request_key: key
      }) : await api.post(`/api/ticket/refund/payouts/${query.data?.items[0]?.id}/review`, {
        status,
        reference,
        evidence,
        claw_back_commission: clawback
      });
      if (!response.data.success) throw new Error(response.data.message);
      setKey(crypto.randomUUID());
      query.reload();
      onUpdated?.();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : t('Operation failed'));
    } finally {
      setBusy(false);
    }
  }
  const attempt = query.data?.items[0];
  const editable = !!user && [10, 100].includes(user.role);
  const payoutLabels = {
    succeeded: 'Completed',
    outcome_unknown: 'Payment outcome unknown',
    processing: 'Processing',
    failed: 'Failed'
  };
  return <section className='space-y-3 border-t pt-4'>
      <h3 className='font-semibold'>{t('Refund payout')}</h3>
      {query.data && <p>
          {t('Cash refund')}: ¥{(query.data.cash_cents / 100).toFixed(2)} ·{' '}
          {t(payoutLabels[attempt?.status ?? ''] ?? 'Pending')}
        </p>}
      <Button variant='outline' onClick={query.reload}>
        {t('Refresh')}
      </Button>
      {editable && query.data?.refund_status === 1 && (!attempt || attempt.status === 'failed') && <Button disabled={busy || !query.data.ready} onClick={() => send('start')}>
            {t('Claim manual payout')}
          </Button>}
      {editable && attempt && ['processing', 'outcome_unknown'].includes(attempt.status) && <div className='space-y-3'>
            <label className='flex items-center gap-2'>
              <input type='checkbox' checked={clawback} disabled={busy} onChange={event => setClawback(event.target.checked)} />
              {t('Claw back inviter commission')}
            </label>
            <label className='block space-y-1'>
              <span>{t('Payment receipt reference')}</span>
              <input className={inputClass} value={reference} maxLength={128} disabled={busy} onChange={event => setReference(event.target.value)} />
            </label>
            <label className='block space-y-1'>
              <span>{t('Payment evidence')}</span>
              <textarea className={inputClass} value={evidence} maxLength={4000} disabled={busy} onChange={event => setEvidence(event.target.value)} />
            </label>
            <div className='flex flex-wrap gap-2'>
              <Button disabled={busy || !reference.trim() || evidence.trim().length < 10} onClick={() => send('succeeded')}>
                {t('Confirm paid refund')}
              </Button>
              <Button variant='outline' disabled={busy || evidence.trim().length < 10} onClick={() => send('outcome_unknown')}>
                {t('Record uncertain outcome')}
              </Button>
              <Button variant='outline' disabled={busy || evidence.trim().length < 10} onClick={() => send('failed')}>
                {t('Confirm no payment occurred')}
              </Button>
            </div>
          </div>}
      {attempt?.evidence && <p className='break-words whitespace-pre-wrap'>
          {attempt.payment_reference} · {attempt.evidence}
        </p>}
      {(error || query.error) && <p role='alert' className='text-red-600'>
          {t(error || query.error)}
        </p>}
    </section>;
}
const phaseLabels = {
  documents_required: 'Original invoice evidence required',
  ready: 'Ready for correction',
  red_processing: 'Red invoice processing',
  reissue_pending: 'Replacement invoice pending',
  completed: 'Completed',
  cancelled: 'Cancelled',
  submitted: 'Pending',
  applied: 'Approved',
  rejected: 'Rejected',
  waiting_settlement: 'Awaiting settlement verification',
  needs_reconciliation: 'Financial reconciliation required',
  awaiting_owner_confirmation: 'Awaiting owner confirmation',
  reserved: 'Reserved',
  closed: 'Closed'
};
const blockerLabels = {
  liability_pending: 'Awaiting settlement verification',
  legacy_payment_snapshot_missing: 'Legacy payment evidence missing',
  unsupported_currency: 'Unsupported currency',
  legacy_refund_unallocated: 'Historical refund allocation missing',
  ledger_mismatch: 'Ledger difference requires evidence'
};
function useFinancialQuery(url) {
  const [data, setData] = useState(null);
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    api.get(url, {
      signal: controller.signal,
      disableDuplicate: true,
      skipErrorHandler: true,
      skipBusinessError: true
    }).then(response => {
      if (!response.data.success) throw new Error(response.data.message);
      if (!controller.signal.aborted) {
        setData(response.data.data);
        setError('');
      }
    }).catch(failure => {
      if (!controller.signal.aborted) setError(failure.message);
    });
    return () => controller.abort();
  }, [url, revision]);
  return {
    data,
    error,
    reload: () => setRevision(value => value + 1)
  };
}
export function CorrectionWorkflow({
  correction: initialCorrection,
  onUpdated,
  editable = true
}) {
  const {
    t
  } = useTranslation();
  const [correction, setCorrection] = useState(initialCorrection);
  const [documents, setDocuments] = useState(correction.documents?.length ? correction.documents.map(doc => ({
    ...doc,
    localId: crypto.randomUUID()
  })) : [{
    localId: crypto.randomUUID(),
    issuer: '',
    original_reference: '',
    original_cents: correction.original_cents,
    reduction_cents: correction.original_cents - correction.reissue_cents,
    red_reference: '',
    reissue_reference: ''
  }]);
  const [evidence, setEvidence] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const phase = correction.phase || correction.status;
  const preparing = correction.status === 'pending';
  const complete = correction.status === 'completed' || correction.status === 'cancelled';
  async function transition(action) {
    if (busy || !editable) return;
    setBusy(true);
    setError('');
    try {
      const response = await api.post(`/api/ticket/refund/invoice-corrections/${correction.id}/transition`, {
        action,
        documents,
        evidence
      });
      if (!response.data.success) throw new Error(response.data.message);
      setCorrection(response.data.data);
      setDocuments(response.data.data.documents.map(doc => ({
        ...doc,
        localId: crypto.randomUUID()
      })));
      setEvidence('');
      onUpdated?.(response.data.data.id, response.data.data.status);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : t('Operation failed'));
    } finally {
      setBusy(false);
    }
  }
  if (correction.version !== 1) {
    return <section className='space-y-2 border-t pt-4'>
        <h4>
          {t('Invoice correction')} #{correction.invoice_ticket_id} ·{' '}
          {t(phaseLabels[correction.status] ?? 'Processing')}
        </h4>
        <p>
          {correction.original_reference} · {correction.red_reference} ·{' '}
          {correction.reissue_reference}
        </p>
        <p className='break-words whitespace-pre-wrap'>{correction.evidence}</p>
        {correction.refund_ticket_id > 0 && <a href={`/console/ticket_admin/${correction.refund_ticket_id}`}>
            {t('Refund')} #{correction.refund_ticket_id}
          </a>}
      </section>;
  }
  return <section className='min-w-0 space-y-3 border-t pt-4'>
      <h4 className='font-semibold'>
        {t('Invoice correction')} #{correction.invoice_ticket_id} ·{' '}
        {t(phaseLabels[phase] ?? phase)}
      </h4>
      <p>
        {t('Original invoice amount')}: ¥
        {(correction.original_cents / 100).toFixed(2)} ·{' '}
        {t('Reissued invoice amount')}: ¥
        {(correction.reissue_cents / 100).toFixed(2)}
      </p>
      <div className='space-y-4'>
        {documents.map((doc, index) => <fieldset key={doc.localId} disabled={busy || !editable || complete} className='grid min-w-0 gap-3 border-b pb-3 sm:grid-cols-2'>
            {['issuer', 'original_reference', 'original_cents', 'reduction_cents', 'red_reference', 'reissue_reference'].map(field => {
          const numeric = field === 'original_cents' || field === 'reduction_cents';
          const label = {
            issuer: 'Invoice issuer',
            original_reference: 'Original invoice reference',
            original_cents: 'Original amount (cents)',
            reduction_cents: 'Reduction amount (cents)',
            red_reference: 'Red invoice reference',
            reissue_reference: 'Reissued invoice reference'
          }[field];
          let disabled = !preparing;
          if (field === 'red_reference') {
            disabled = phase !== 'red_processing';
          }
          if (field === 'reissue_reference') {
            disabled = phase !== 'reissue_pending' || doc.original_cents === doc.reduction_cents;
          }
          return <label key={field} className='min-w-0 space-y-1 text-sm'>
                  <span>{t(label)}</span>
                  <input className={inputClass} aria-label={`${t(label)} ${index + 1}`} type={numeric ? 'number' : 'text'} min={numeric ? 0 : undefined} step={numeric ? 1 : undefined} maxLength={numeric ? undefined : 255} value={doc[field]} disabled={disabled} onChange={event => setDocuments(rows => rows.map((row, i) => i === index ? {
              ...row,
              [field]: numeric ? Number(event.target.value) : event.target.value
            } : row))} />
                </label>;
        })}
            {preparing && documents.length > 1 && <Button variant='outline' onClick={() => setDocuments(rows => rows.filter((_, i) => i !== index))}>
                {t('Remove')}
              </Button>}
          </fieldset>)}
      </div>
      {editable && !complete && <>
          {preparing && <Button variant='outline' disabled={busy || documents.length >= 100} onClick={() => setDocuments(rows => [...rows, {
        localId: crypto.randomUUID(),
        issuer: '',
        original_reference: '',
        original_cents: 0,
        reduction_cents: 0,
        red_reference: '',
        reissue_reference: ''
      }])}>
              {t('Add invoice document')}
            </Button>}
          <label className='block space-y-1'>
            <span>{t('Correction evidence')}</span>
            <textarea className={inputClass} value={evidence} maxLength={4000} disabled={busy} onChange={event => setEvidence(event.target.value)} />
          </label>
          <div className='flex flex-wrap gap-2'>
            {preparing && <Button disabled={busy || !evidence.trim()} onClick={() => transition('prepare')}>
                {t('Save invoice evidence')}
              </Button>}
            {phase === 'ready' && <Button disabled={busy} onClick={() => transition('start')}>
                {t('Start correction')}
              </Button>}
            {phase === 'red_processing' && <Button disabled={busy || !evidence.trim()} onClick={() => transition('red')}>
                {t('Register red invoice')}
              </Button>}
            {phase === 'reissue_pending' && <Button disabled={busy || !evidence.trim()} onClick={() => transition('reissue')}>
                {t('Register replacement invoice')}
              </Button>}
            {preparing && !correction.refund_ticket_id && <Button variant='outline' disabled={busy || !evidence.trim()} onClick={() => transition('cancel')}>
                {t('Cancel correction')}
              </Button>}
          </div>
        </>}
      {correction.evidence && <p className='text-sm break-words whitespace-pre-wrap'>
          {correction.evidence}
        </p>}
      {error && <p role='alert' className='text-red-600'>
          {t(error)}
        </p>}
    </section>;
}
export function InvoiceCorrectionPanel({
  ticketId
}) {
  const {
    t
  } = useTranslation();
  const query = useFinancialQuery(`/api/ticket/invoice/${ticketId}/corrections`);
  const [reason, setReason] = useState('');
  const [reissue, setReissue] = useState(true);
  const [key, setKey] = useState(() => crypto.randomUUID());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function create() {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      const response = await api.post(`/api/ticket/invoice/${ticketId}/corrections`, {
        request_key: key,
        reason,
        reissue
      });
      if (!response.data.success) throw new Error(response.data.message);
      setKey(crypto.randomUUID());
      setReason('');
      query.reload();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : t('Operation failed'));
    } finally {
      setBusy(false);
    }
  }
  const active = query.data?.corrections.some(row => row.status === 'pending' || row.status === 'processing');
  return <section className='min-w-0 space-y-3 border-t pt-4'>
      <h3 className='font-semibold'>{t('Invoice correction')}</h3>
      {!active && query.data && <div className='space-y-3'>
          <label className='block space-y-1'>
            <span>{t('Reason')}</span>
            <textarea className={inputClass} value={reason} maxLength={2000} onChange={event => setReason(event.target.value)} disabled={busy} />
          </label>
          <label className='flex items-center gap-2'>
            <input type='checkbox' checked={reissue} disabled={busy} onChange={event => setReissue(event.target.checked)} />
            {t('Reissue for the same amount')}
          </label>
          <Button disabled={busy || reason.trim().length < 5} onClick={create}>
            {t('Request invoice correction')}
          </Button>
        </div>}
      {query.data?.corrections.map(row => <CorrectionWorkflow key={`${row.id}:${row.phase}:${row.status}`} correction={row} editable={row.version === 1} onUpdated={query.reload} />)}
      {(error || query.error) && <p role='alert' className='text-red-600'>
          {t(error || query.error)}
        </p>}
    </section>;
}
export function ReconciliationPanel({
  ticketId
}) {
  const query = useFinancialQuery(`/api/ticket/refund/${ticketId}/reconciliation`);
  const {
    t
  } = useTranslation();
  return <section className='min-w-0 space-y-3 border-t pt-4'>
      <h3 className='font-semibold'>{t('Refund reconciliation')}</h3>
      <Button variant='outline' onClick={query.reload}>
        {t('Refresh')}
      </Button>
      {query.error && <p role='alert' className='text-red-600'>
          {t(query.error)}
        </p>}
      {query.data && <ReconciliationEditor key={query.data.revision} ticketId={ticketId} report={query.data} onUpdated={query.reload} />}
    </section>;
}
function ReconciliationEditor({
  ticketId,
  report,
  onUpdated
}) {
  const {
    t
  } = useTranslation();
  const [userState] = useContext(UserContext);
  const user = userState.user;
  const money = createFinanceMoney(report.quota_per_unit, report.usd_exchange_rate);
  const [payments, setPayments] = useState(report.orders.filter(row => row.checkout_version === 0).map(row => ({
    top_up_id: row.id,
    paid_cents: cashMoney.input(Math.round(row.money * 100)),
    tax_cents: '0',
    total_quota: row.quota,
    bonus_quota: '0'
  })));
  const [grant, setGrant] = useState('0');
  const parsedPayments = payments.map(row => ({
    top_up_id: row.top_up_id,
    paid_cents: cashMoney.parse(row.paid_cents),
    tax_cents: cashMoney.parse(row.tax_cents),
    paid_quota: row.total_quota - (money.parse(row.bonus_quota) ?? row.total_quota),
    bonus_quota: money.parse(row.bonus_quota)
  }));
  const parsedGrant = money.parse(grant);
  const amountsValid = money.valid && parsedGrant !== null && parsedPayments.every(row => row.paid_cents !== null && row.tax_cents !== null && row.paid_quota > 0 && row.tax_cents < row.paid_cents && row.bonus_quota !== null);
  const [evidence, setEvidence] = useState('');
  const [reviewReason, setReviewReason] = useState('');
  const [key, setKey] = useState(() => crypto.randomUUID());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function send(url, payload) {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      const response = await api.post(url, payload);
      if (!response.data.success) throw new Error(response.data.message);
      setKey(crypto.randomUUID());
      onUpdated();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : t('Operation failed'));
    } finally {
      setBusy(false);
    }
  }
  return <div className='space-y-4'>
      <p>{t(phaseLabels[report.stage] ?? report.stage)}</p>
      <p>
        {t('Paid credit value (CNY)')}: {money.format(report.paid_quota)} ·{' '}
        {t('Noncash credit value (CNY)')}: {money.format(report.noncash_quota)}{' '}
        · {t('Unexplained difference (CNY)')}: {money.format(report.difference)}
      </p>
      {!money.valid && <p role='alert'>{t('Currency conversion settings unavailable')}</p>}
      <ul>
        {report.blockers.map(blocker => <li key={`${blocker.code}:${blocker.order_id ?? ''}`}>
            {t(blockerLabels[blocker.code] ?? blocker.code)}{' '}
            {blocker.order_id ? `#${blocker.order_id}` : ''}
          </li>)}
      </ul>
      {report.stage !== 'reserved' && report.stage !== 'closed' && <fieldset disabled={busy || !money.valid} className='space-y-3'>
          {payments.map((row, index) => <div key={row.top_up_id} className='grid gap-2 border-b pb-3 sm:grid-cols-2'>
              <p className='sm:col-span-2'>
                {t('Order number')} #{row.top_up_id}
              </p>
              <p>
                {t('Actual paid amount (CNY)')}:{' '}
                {cashMoney.format(parsedPayments[index].paid_cents ?? 0)}
              </p>
              <p>
                {t('Order credit total (CNY)')}: {money.format(row.total_quota)}
              </p>
              <p className='sm:col-span-2'>
                {t('Purchased credit value (CNY)')}:{' '}
                {money.format(parsedPayments[index].paid_quota)}
              </p>
              {['tax_cents', 'bonus_quota'].map(field => <label key={field} className='space-y-1'>
                  <span>
                    {t({
              tax_cents: 'Collected fee (CNY)',
              bonus_quota: 'Included bonus value (CNY)'
            }[field])}
                  </span>
                  <input className={inputClass} type='number' min={0} step='any' value={row[field]} onChange={event => setPayments(rows => rows.map((entry, i) => i === index ? {
            ...entry,
            [field]: event.target.value
          } : entry))} />
                </label>)}
            </div>)}
          <p>
            {t('Purchased value equals order total minus included bonus. Cash refunds use the recorded actual payment.')}
          </p>
          <label className='block space-y-1'>
            <span>{t('Verified additional noncash value (CNY)')}</span>
            <input className={inputClass} type='number' min={0} step='any' value={grant} onChange={event => setGrant(event.target.value)} />
          </label>
          <p>
            {t('Unexplained differences are not automatically gifts. Only enter additional gifts supported by records; other differences remain pending investigation.')}
          </p>
          <label className='block space-y-1'>
            <span>{t('Source evidence (at least 10 characters)')}</span>
            <textarea className={inputClass} value={evidence} maxLength={4000} onChange={event => setEvidence(event.target.value)} />
          </label>
          <Button disabled={busy || !money.valid} onClick={() => {
        if (!amountsValid || parsedGrant === null || parsedGrant > 2147483647 || parsedGrant > 0 && BigInt(parsedGrant) > BigInt(report.difference)) {
          setError('Check amounts: fees must be below payment, included bonus below order total, and additional gifts cannot exceed the unexplained difference.');
          return;
        }
        if ([...evidence.trim()].length < 10) {
          setError('Provide source evidence of at least 10 characters, including receipt or grant record references.');
          return;
        }
        if (!payments.length && !parsedGrant) {
          setError('No new financial evidence to submit.');
          return;
        }
        send(`/api/ticket/refund/${ticketId}/reconciliation`, {
          request_key: key,
          revision: report.revision,
          plan: {
            payments: parsedPayments,
            noncash_grant_quota: parsedGrant,
            evidence
          }
        });
      }}>
            {t('Submit for review')}
          </Button>
        </fieldset>}
      {report.reviews.map(review => <section key={review.id} className='space-y-2 border-t pt-3'>
          <h4>
            #{review.id} · {t(phaseLabels[review.status] ?? review.status)}
          </h4>
          <p className='break-words whitespace-pre-wrap'>
            {review.plan.evidence}
          </p>
          <p>
            {t('Verified additional noncash value (CNY)')}:{' '}
            {money.format(review.plan.noncash_grant_quota)}
          </p>
          {review.plan.payments.map(row => <p key={row.top_up_id}>
              #{row.top_up_id} · {t('Actual paid amount (CNY)')}:{' '}
              {cashMoney.format(row.paid_cents)} · {t('Collected fee (CNY)')}:{' '}
              {cashMoney.format(row.tax_cents)} ·{' '}
              {t('Purchased credit value (CNY)')}:{' '}
              {money.format(row.paid_quota)} · {t('Included bonus value (CNY)')}
              : {money.format(row.bonus_quota)}
            </p>)}
          {review.status === 'submitted' && user && [10, 100].includes(user.role) && user.id !== review.proposed_by && <>
                <label className='block space-y-1'>
                  <span>{t('Review evidence')}</span>
                  <textarea className={inputClass} value={reviewReason} maxLength={4000} onChange={event => setReviewReason(event.target.value)} disabled={busy} />
                </label>
                <div className='flex flex-wrap gap-2'>
                  <Button disabled={busy || !reviewReason.trim()} onClick={() => send(`/api/ticket/refund/reconciliations/${review.id}/review`, {
            approve: true,
            reason: reviewReason
          })}>
                    {t('Approve reconciliation')}
                  </Button>
                  <Button variant='outline' disabled={busy || !reviewReason.trim()} onClick={() => send(`/api/ticket/refund/reconciliations/${review.id}/review`, {
            approve: false,
            reason: reviewReason
          })}>
                    {t('Reject')}
                  </Button>
                </div>
              </>}
        </section>)}
      {error && <p role='alert' className='text-red-600'>
          {t(error)}
        </p>}
    </div>;
}
