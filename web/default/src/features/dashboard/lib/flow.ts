/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type {
  DashboardFlowGraph,
  DashboardFlowLink,
  DashboardFlowNode,
  FlowBuildOptions,
  FlowFilterOptions,
  FlowMetric,
  FlowNodeKind,
  FlowOverflowMode,
  FlowQuotaDataItem,
  FlowRole,
  FlowSummary,
  ProcessedFlowData,
} from '@/features/dashboard/types'

import { getDashboardChartColors } from './charts'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type VChartSpec = Record<string, any>

type FlowMetrics = {
  quota: number
  tokens: number
  requests: number
}

type FlowSankeyLabels = {
  quota: string
  tokens: string
  requests: string
  share: string
}

type FlowPathNode = {
  id: string
  label: string
  kind: FlowNodeKind
}

type PreparedFlowPath = {
  path: FlowPathNode[]
  metrics: FlowMetrics
}

type FlowNodeRank = {
  node: FlowPathNode
  value: number
}

type FlowPathContext = {
  deletedTokenLabel?: (tokenId: number) => string
}

type FlowGraphOptions = {
  topNodeLimit?: number
  overflowMode?: FlowOverflowMode
  otherNodeLabel?: (kind: FlowNodeKind) => string
}

const EMPTY_FLOW_PATH_CONTEXT: FlowPathContext = {}

const DEFAULT_FLOW_ROLE: FlowRole = 'user'

const DEFAULT_FLOW_OVERFLOW_MODE: FlowOverflowMode = 'aggregate'

const DEFAULT_FLOW_SANKEY_LABELS: FlowSankeyLabels = {
  quota: 'Quota',
  tokens: 'Tokens',
  requests: 'Requests',
  share: 'Share',
}

const DEFAULT_FLOW_CHART_COLOR = '#1664FF'

const OTHER_FLOW_NODE_IDS: Record<FlowNodeKind, string> = {
  user: 'user:__other__',
  node: 'node:__other__',
  token: 'token:__other__',
  group: 'group:__other__',
  model: 'model:__other__',
  channel: 'channel:__other__',
}

const DEFAULT_OTHER_FLOW_NODE_LABELS: Record<FlowNodeKind, string> = {
  user: 'Other users',
  node: 'Other nodes',
  token: 'Other tokens',
  group: 'Other groups',
  model: 'Other models',
  channel: 'Other channels',
}

function numberValue(value: unknown): number {
  const n = Number(value)
  return Number.isFinite(n) ? n : 0
}

function rowMetrics(row: FlowQuotaDataItem): FlowMetrics {
  return {
    quota: numberValue(row.quota),
    tokens: numberValue(row.token_used),
    requests: numberValue(row.count),
  }
}

function metricValue(metrics: FlowMetrics, metric: FlowMetric): number {
  if (metric === 'requests') return metrics.requests
  if (metric === 'tokens') return metrics.tokens
  return metrics.quota
}

function userNode(row: FlowQuotaDataItem): FlowPathNode {
  const userID = numberValue(row.user_id)
  return {
    id: userID > 0 ? `user:${userID}` : `user:${row.username || 'unknown'}`,
    label: row.username || (userID > 0 ? `user-${userID}` : 'Unknown User'),
    kind: 'user',
  }
}

function nodeNameNode(row: FlowQuotaDataItem): FlowPathNode {
  const nodeName = row.node_name || 'default-node'
  return {
    id: `node:${nodeName}`,
    label: nodeName,
    kind: 'node',
  }
}

function tokenNode(row: FlowQuotaDataItem, ctx: FlowPathContext): FlowPathNode {
  const tokenID = numberValue(row.token_id)
  return {
    id:
      tokenID > 0 ? `token:${tokenID}` : `token:${row.token_name || 'unknown'}`,
    label: row.token_name || deletedTokenLabel(tokenID, ctx),
    kind: 'token',
  }
}

function deletedTokenLabel(tokenID: number, ctx: FlowPathContext): string {
  if (tokenID <= 0) return 'Unknown Token'
  return ctx.deletedTokenLabel?.(tokenID) ?? `token-${tokenID}`
}

function groupNode(row: FlowQuotaDataItem): FlowPathNode {
  const useGroup = row.use_group || 'unknown'
  return {
    id: `group:${useGroup}`,
    label: useGroup,
    kind: 'group',
  }
}

function modelNode(row: FlowQuotaDataItem): FlowPathNode {
  const model = row.model_name || 'unknown'
  return {
    id: `model:${model}`,
    label: row.model_name || 'Unknown Model',
    kind: 'model',
  }
}

function channelNode(row: FlowQuotaDataItem): FlowPathNode {
  const channelID = numberValue(row.channel_id)
  return {
    id:
      channelID > 0
        ? `channel:${channelID}`
        : `channel:${row.channel_name || 'unknown'}`,
    label:
      row.channel_name || (channelID > 0 ? `channel-${channelID}` : 'Unknown'),
    kind: 'channel',
  }
}

const NODE_BUILDERS: Record<
  FlowNodeKind,
  (row: FlowQuotaDataItem, ctx: FlowPathContext) => FlowPathNode
> = {
  user: userNode,
  node: nodeNameNode,
  token: tokenNode,
  group: groupNode,
  model: modelNode,
  channel: channelNode,
}

const ROLE_FLOW_STAGES: Record<FlowRole, FlowNodeKind[]> = {
  root: ['user', 'node', 'token', 'group', 'model', 'channel'],
  admin: ['user', 'group', 'model', 'channel'],
  user: ['token', 'group', 'model'],
}

// A Sankey needs at least two columns to draw any link, so hiding stages can
// never collapse the path below this many columns.
const MIN_FLOW_STAGES = 2

export function getFlowStages(role: FlowRole): FlowNodeKind[] {
  return ROLE_FLOW_STAGES[role] ?? ROLE_FLOW_STAGES.user
}

function resolveVisibleStages(
  role: FlowRole,
  visibleStages?: FlowNodeKind[]
): FlowNodeKind[] {
  const stages = getFlowStages(role)
  if (!visibleStages) return stages
  const visible = new Set(visibleStages)
  const filtered = stages.filter((stage) => visible.has(stage))
  return filtered.length >= MIN_FLOW_STAGES ? filtered : stages
}

function flowPathForStages(
  row: FlowQuotaDataItem,
  stages: FlowNodeKind[],
  ctx: FlowPathContext = EMPTY_FLOW_PATH_CONTEXT
): FlowPathNode[] {
  return stages.map((stage) => NODE_BUILDERS[stage](row, ctx))
}

function colorAt(index: number, palette?: readonly string[]): string {
  const colors =
    palette && palette.length > 0 ? palette : getDashboardChartColors(index + 1)
  if (colors.length === 0) return DEFAULT_FLOW_CHART_COLOR
  return colors[index % colors.length] ?? DEFAULT_FLOW_CHART_COLOR
}

function colorPalette(
  colorCount: number,
  palette?: readonly string[]
): readonly string[] {
  if (palette && palette.length > 0) return palette
  const colors = getDashboardChartColors(colorCount)
  return colors.length > 0 ? colors : [DEFAULT_FLOW_CHART_COLOR]
}

function alphaColor(
  color: string,
  alpha: number
): { color: string; alpha: number } {
  const normalized = color.trim()
  const hex = normalized.startsWith('#') ? normalized.slice(1) : normalized
  if (!/^[0-9a-f]{6}$/i.test(hex)) {
    return { color: normalized, alpha }
  }

  const value = Number.parseInt(hex, 16)
  const red = (value >> 16) & 255
  const green = (value >> 8) & 255
  const blue = value & 255
  return {
    color: `rgba(${red}, ${green}, ${blue}, ${alpha.toFixed(2)})`,
    alpha: 1,
  }
}

function stableColorMap(
  keys: string[],
  palette?: readonly string[]
): Map<string, string> {
  const map = new Map<string, string>()
  const uniqueKeys = Array.from(new Set(keys))
  const colors = colorPalette(uniqueKeys.length, palette)
  uniqueKeys.forEach((key, index) => {
    map.set(key, colorAt(index, colors))
  })
  return map
}

function filterRows(
  rows: FlowQuotaDataItem[],
  options: FlowBuildOptions = {}
): FlowQuotaDataItem[] {
  const selectedUsers = new Set(options.selectedUsers ?? [])
  if (selectedUsers.size === 0) return rows
  return rows.filter((row) => selectedUsers.has(userNode(row).id))
}

function addNode(
  map: Map<string, DashboardFlowNode>,
  pathNode: FlowPathNode,
  metrics: FlowMetrics,
  metric: FlowMetric,
  color: string,
  colorKey: string
): void {
  const previous = map.get(pathNode.id) ?? {
    id: pathNode.id,
    label: pathNode.label,
    kind: pathNode.kind,
    value: 0,
    requests: 0,
    quota: 0,
    tokens: 0,
    color,
    colorKey,
  }
  previous.value += metricValue(metrics, metric)
  previous.requests += metrics.requests
  previous.quota += metrics.quota
  previous.tokens += metrics.tokens
  map.set(pathNode.id, previous)
}

function addLink(
  map: Map<string, DashboardFlowLink>,
  source: FlowPathNode,
  target: FlowPathNode,
  metrics: FlowMetrics,
  metric: FlowMetric,
  color: string,
  colorKey: string
): void {
  const key = `${source.id}\u0000${target.id}`
  const previous = map.get(key) ?? {
    source: source.id,
    target: target.id,
    value: 0,
    requests: 0,
    quota: 0,
    tokens: 0,
    sourceLabel: source.label,
    targetLabel: target.label,
    color,
    linkColor: color,
    linkAlpha: 1,
    hoverColor: color,
    colorKey,
    share: 0,
  }
  previous.value += metricValue(metrics, metric)
  previous.requests += metrics.requests
  previous.quota += metrics.quota
  previous.tokens += metrics.tokens
  map.set(key, previous)
}

function assignLinkDisplayColors(links: DashboardFlowLink[]): void {
  const linksBySource = new Map<string, DashboardFlowLink[]>()
  for (const link of links) {
    const sourceLinks = linksBySource.get(link.source) ?? []
    sourceLinks.push(link)
    linksBySource.set(link.source, sourceLinks)
  }

  for (const sourceLinks of linksBySource.values()) {
    const sortedLinks = [...sourceLinks].sort(
      (a, b) =>
        b.value - a.value || linkStableKey(a).localeCompare(linkStableKey(b))
    )
    const denominator = Math.max(sortedLinks.length - 1, 1)
    sortedLinks.forEach((link, index) => {
      const alpha =
        sortedLinks.length === 1 ? 0.34 : 0.24 + (index / denominator) * 0.2
      const displayColor = alphaColor(link.color, alpha)
      link.linkColor = displayColor.color
      link.linkAlpha = displayColor.alpha
      link.hoverColor = link.color
    })
  }
}

function byValueThenLabel<T extends { value: number; label: string }>(
  a: T,
  b: T
): number {
  return b.value - a.value || a.label.localeCompare(b.label)
}

function linkStableKey(link: Pick<DashboardFlowLink, 'source' | 'target'>) {
  return `${link.source}\u0000${link.target}`
}

function byLinkDrawPriority(
  a: DashboardFlowLink,
  b: DashboardFlowLink
): number {
  return b.value - a.value || linkStableKey(a).localeCompare(linkStableKey(b))
}

function buildSummary(rows: FlowQuotaDataItem[]): FlowSummary {
  return rows.reduce<FlowSummary>(
    (summary, row) => {
      const metrics = rowMetrics(row)
      summary.quota += metrics.quota
      summary.tokens += metrics.tokens
      summary.requests += metrics.requests
      return summary
    },
    {
      quota: 0,
      tokens: 0,
      requests: 0,
    }
  )
}

function normalizeTopNodeLimit(limit?: number): number | undefined {
  if (limit === undefined) return undefined
  const parsed = Math.floor(limit)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined
}

function otherFlowNode(
  kind: FlowNodeKind,
  labeler?: (kind: FlowNodeKind) => string
): FlowPathNode {
  return {
    id: OTHER_FLOW_NODE_IDS[kind],
    label: labeler?.(kind) ?? DEFAULT_OTHER_FLOW_NODE_LABELS[kind],
    kind,
  }
}

function buildTopNodeSets(
  rows: FlowQuotaDataItem[],
  metric: FlowMetric,
  stages: FlowNodeKind[],
  limit: number | undefined,
  ctx: FlowPathContext
): Map<FlowNodeKind, Set<string>> | undefined {
  if (!limit) return undefined

  const totals = new Map<FlowNodeKind, Map<string, FlowNodeRank>>()
  for (const stage of stages) {
    totals.set(stage, new Map())
  }

  for (const row of rows) {
    const metrics = rowMetrics(row)
    const value = metricValue(metrics, metric)
    const path = flowPathForStages(row, stages, ctx)
    for (const node of path) {
      const stageTotals = totals.get(node.kind)
      if (!stageTotals) continue
      const current = stageTotals.get(node.id) ?? { node, value: 0 }
      current.value += value
      stageTotals.set(node.id, current)
    }
  }

  const topSets = new Map<FlowNodeKind, Set<string>>()
  for (const [kind, stageTotals] of totals) {
    const topIds = Array.from(stageTotals.values())
      .sort(
        (a, b) =>
          b.value - a.value ||
          a.node.label.localeCompare(b.node.label) ||
          a.node.id.localeCompare(b.node.id)
      )
      .slice(0, limit)
      .map((rank) => rank.node.id)
    topSets.set(kind, new Set(topIds))
  }

  return topSets
}

function isTopFlowNode(
  node: FlowPathNode,
  topNodeSets?: Map<FlowNodeKind, Set<string>>
): boolean {
  const topNodes = topNodeSets?.get(node.kind)
  return !topNodes || topNodes.has(node.id)
}

function applyTopNodeLimit(
  path: FlowPathNode[],
  topNodeSets: Map<FlowNodeKind, Set<string>> | undefined,
  mode: FlowOverflowMode,
  labeler?: (kind: FlowNodeKind) => string
): FlowPathNode[] | undefined {
  if (!topNodeSets) return path
  const containsOverflowNode = path.some(
    (node) => !isTopFlowNode(node, topNodeSets)
  )
  if (!containsOverflowNode) return path
  if (mode === 'hide') return undefined
  return path.map((node) =>
    isTopFlowNode(node, topNodeSets) ? node : otherFlowNode(node.kind, labeler)
  )
}

function buildFlowGraph(
  rows: FlowQuotaDataItem[],
  metric: FlowMetric,
  role: FlowRole,
  palette?: readonly string[],
  visibleStages?: FlowNodeKind[],
  ctx: FlowPathContext = EMPTY_FLOW_PATH_CONTEXT,
  options: FlowGraphOptions = {}
): DashboardFlowGraph {
  const stages = resolveVisibleStages(role, visibleStages)
  const topNodeSets = buildTopNodeSets(
    rows,
    metric,
    stages,
    normalizeTopNodeLimit(options.topNodeLimit),
    ctx
  )
  const overflowMode = options.overflowMode ?? DEFAULT_FLOW_OVERFLOW_MODE
  const preparedPaths: PreparedFlowPath[] = []

  for (const row of rows) {
    const path = applyTopNodeLimit(
      flowPathForStages(row, stages, ctx),
      topNodeSets,
      overflowMode,
      options.otherNodeLabel
    )
    if (!path) continue
    preparedPaths.push({ path, metrics: rowMetrics(row) })
  }

  const nodes = new Map<string, DashboardFlowNode>()
  const links = new Map<string, DashboardFlowLink>()
  const colors = stableColorMap(
    preparedPaths
      .map((prepared) => prepared.path[0]?.id)
      .filter((id): id is string => Boolean(id))
      .sort((a, b) => a.localeCompare(b)),
    palette
  )

  for (const prepared of preparedPaths) {
    const { path, metrics } = prepared
    const root = path[0]
    if (!root) continue
    const color = colors.get(root.id) ?? colorAt(0, palette)

    for (const node of path) {
      addNode(nodes, node, metrics, metric, color, root.id)
    }
    for (let i = 0; i < path.length - 1; i++) {
      const source = path[i]
      const target = path[i + 1]
      if (!source || !target) continue
      addLink(links, source, target, metrics, metric, color, root.id)
    }
  }

  const flowLinks = Array.from(links.values()).sort(
    (a, b) =>
      a.source.localeCompare(b.source) || a.target.localeCompare(b.target)
  )
  const firstStepSources = new Set(
    preparedPaths
      .map((prepared) => prepared.path[0]?.id)
      .filter((id): id is string => Boolean(id))
  )
  const total = flowLinks
    .filter((link) => firstStepSources.has(link.source))
    .reduce((sum, link) => sum + link.value, 0)
  for (const link of flowLinks) {
    link.share = total > 0 ? link.value / total : 0
  }
  assignLinkDisplayColors(flowLinks)

  return {
    nodes: Array.from(nodes.values()).sort(byValueThenLabel),
    links: flowLinks,
  }
}

function formatNumber(value: number): string {
  return Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(
    value
  )
}

export function buildFlowFilterOptions(
  rows: FlowQuotaDataItem[],
  metric: FlowMetric = 'quota',
  palette?: readonly string[]
): FlowFilterOptions {
  const users = new Map<
    string,
    {
      label: string
      value: number
      color: string
    }
  >()
  const colors = stableColorMap(
    rows.map((row) => userNode(row).id).sort((a, b) => a.localeCompare(b)),
    palette
  )

  for (const row of rows) {
    const user = userNode(row)
    if (!row.user_id && !row.username) continue
    const metrics = rowMetrics(row)
    const value = metricValue(metrics, metric)
    const current = users.get(user.id) ?? {
      label: user.label,
      value: 0,
      color: colors.get(user.id) ?? colorAt(0, palette),
    }
    current.value += value
    users.set(user.id, current)
  }

  return {
    users: Array.from(users.entries())
      .map(([value, user]) => ({
        value,
        label: user.label,
        valueLabel: formatNumber(user.value),
        valueRaw: user.value,
        color: user.color,
      }))
      .sort(
        (a, b) => b.valueRaw - a.valueRaw || a.label.localeCompare(b.label)
      ),
  }
}

export function buildDashboardFlowData(
  rows: FlowQuotaDataItem[],
  metric: FlowMetric = 'quota',
  options: FlowBuildOptions = {}
): ProcessedFlowData {
  const role = options.role ?? DEFAULT_FLOW_ROLE
  const filteredRows = filterRows(rows, options)
  const palette = options.colorPalette

  return {
    summary: buildSummary(filteredRows),
    flow: buildFlowGraph(
      filteredRows,
      metric,
      role,
      palette,
      options.visibleStages,
      {
        deletedTokenLabel: options.deletedTokenLabel,
      },
      {
        topNodeLimit: options.topNodeLimit,
        overflowMode: options.overflowMode,
        otherNodeLabel: options.otherNodeLabel,
      }
    ),
    filterOptions: buildFlowFilterOptions(rows, metric, palette),
  }
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object'
    ? (value as Record<string, unknown>)
    : undefined
}

function sankeyDatumSource(
  datum: Record<string, unknown>
): Record<string, unknown> {
  const nested = datum.datum
  if (Array.isArray(nested)) {
    const depth = numberValue(datum.depth)
    return recordValue(nested[depth]) ?? recordValue(nested[0]) ?? datum
  }
  return recordValue(nested) ?? datum
}

function sankeyDatumValue(
  datum: Record<string, unknown>,
  key: string
): unknown {
  if (datum[key] !== undefined) return datum[key]
  return sankeyDatumSource(datum)[key]
}

function isSankeyLinkDatum(datum: Record<string, unknown>): boolean {
  return (
    sankeyDatumValue(datum, 'source') !== undefined &&
    sankeyDatumValue(datum, 'target') !== undefined
  )
}

function tooltipMetricLines(
  valueFormatter: (value: number) => string,
  labels: FlowSankeyLabels
) {
  const metricValue = (datum: Record<string, unknown>, key: string) =>
    numberValue(sankeyDatumValue(datum, key))
  const formattedNumber = (datum: Record<string, unknown>, key: string) =>
    formatNumber(metricValue(datum, key))
  const hasMetric = (datum: Record<string, unknown>, key: string) =>
    metricValue(datum, key) > 0

  return [
    {
      key: labels.quota,
      value: (datum: Record<string, unknown>) =>
        valueFormatter(metricValue(datum, 'quota')),
    },
    {
      key: labels.tokens,
      value: (datum: Record<string, unknown>) =>
        formattedNumber(datum, 'tokens'),
    },
    {
      key: labels.requests,
      value: (datum: Record<string, unknown>) =>
        formattedNumber(datum, 'requests'),
    },
    {
      key: labels.share,
      value: (datum: Record<string, unknown>) =>
        `${(metricValue(datum, 'share') * 100).toFixed(1)}%`,
      visible: (datum: Record<string, unknown>) => hasMetric(datum, 'share'),
    },
  ]
}

export function buildFlowSankeySpec(
  flow: DashboardFlowGraph,
  title: string,
  valueFormatter: (value: number) => string = formatNumber,
  labels: FlowSankeyLabels = DEFAULT_FLOW_SANKEY_LABELS
): VChartSpec {
  return {
    type: 'sankey',
    data: [
      {
        id: 'flow',
        values: [
          {
            nodes: flow.nodes.map((node) => ({
              key: node.id,
              name: node.label,
              rawLabel: node.label,
              kind: node.kind,
              value: node.value,
              requests: node.requests,
              quota: node.quota,
              tokens: node.tokens,
              color: node.color,
              colorKey: node.colorKey,
            })),
            links: flow.links
              .filter((link) => link.value > 0)
              .sort(byLinkDrawPriority)
              .map((link, index) => ({
                source: link.source,
                target: link.target,
                linkKey: linkStableKey(link),
                sourceLabel: link.sourceLabel,
                targetLabel: link.targetLabel,
                value: link.value,
                requests: link.requests,
                quota: link.quota,
                tokens: link.tokens,
                color: link.color,
                linkColor: link.linkColor,
                linkAlpha: link.linkAlpha,
                hoverColor: link.hoverColor,
                colorKey: link.colorKey,
                share: link.share,
                zIndex: index,
              })),
          },
        ],
      },
    ],
    categoryField: 'name',
    sourceField: 'source',
    targetField: 'target',
    valueField: 'value',
    nodeKey: 'key',
    direction: 'horizontal',
    nodeAlign: 'justify',
    crossNodeAlign: 'middle',
    linkSortBy: (
      a: { value?: number; source?: string; target?: string; index?: number },
      b: { value?: number; source?: string; target?: string; index?: number }
    ) =>
      numberValue(b.value) - numberValue(a.value) ||
      `${a.source ?? ''}\u0000${a.target ?? ''}`.localeCompare(
        `${b.source ?? ''}\u0000${b.target ?? ''}`
      ) ||
      numberValue(a.index) - numberValue(b.index),
    nodeGap: 14,
    nodeWidth: 16,
    minLinkHeight: 2,
    minNodeHeight: 8,
    title: {
      visible: false,
      text: title,
    },
    legends: { visible: false },
    label: {
      visible: true,
      position: 'outside',
      limit: 220,
      interactive: false,
      style: {
        fill: '#475569',
        fontSize: 11,
        fontWeight: 600,
      },
    },
    node: {
      interactive: true,
      style: {
        fill: (datum: Record<string, unknown>) =>
          String(sankeyDatumValue(datum, 'color') ?? colorAt(0)),
        fillOpacity: 0.92,
        stroke: 'rgba(148, 163, 184, 0.45)',
        lineWidth: 1,
        cursor: 'pointer',
        pickMode: 'accurate',
      },
      state: {
        hover: {
          fillOpacity: 1,
          stroke: 'rgba(15, 23, 42, 0.68)',
          lineWidth: 1.5,
        },
        selected: {
          fillOpacity: 1,
          stroke: 'rgba(15, 23, 42, 0.68)',
          lineWidth: 1.5,
        },
        blur: {
          fillOpacity: 0.22,
        },
      },
    },
    link: {
      interactive: true,
      style: {
        fill: (datum: Record<string, unknown>) =>
          String(
            sankeyDatumValue(datum, 'linkColor') ??
              sankeyDatumValue(datum, 'color') ??
              colorAt(0)
          ),
        fillOpacity: (datum: Record<string, unknown>) =>
          numberValue(sankeyDatumValue(datum, 'linkAlpha')) || 1,
        cursor: 'pointer',
        pickMode: 'accurate',
        boundsMode: 'accurate',
        zIndex: (datum: Record<string, unknown>) => {
          const zIndex = sankeyDatumValue(datum, 'zIndex')
          if (zIndex !== undefined) return numberValue(zIndex)
          return 1_000_000_000 - numberValue(sankeyDatumValue(datum, 'value'))
        },
      },
      state: {
        hover: {
          fill: (datum: Record<string, unknown>) =>
            String(
              sankeyDatumValue(datum, 'hoverColor') ??
                sankeyDatumValue(datum, 'color') ??
                colorAt(0)
            ),
          fillOpacity: 0.9,
        },
        selected: {
          fill: (datum: Record<string, unknown>) =>
            String(
              sankeyDatumValue(datum, 'hoverColor') ??
                sankeyDatumValue(datum, 'color') ??
                colorAt(0)
            ),
          fillOpacity: 0.9,
        },
        blur: {
          fillOpacity: 0.22,
        },
      },
    },
    emphasis: { enable: false, trigger: 'hover', effect: 'self' },
    tooltip: {
      trigger: 'hover',
      activeType: 'mark',
      dimension: { visible: false },
      group: { visible: false },
      mark: {
        checkOverlap: true,
        positionMode: 'pointer',
        visible: (datum: Record<string, unknown>) =>
          isSankeyLinkDatum(datum) ||
          sankeyDatumValue(datum, 'key') !== undefined,
        title: {
          value: (datum: Record<string, unknown>) => {
            const source = sankeyDatumValue(datum, 'source')
            const target = sankeyDatumValue(datum, 'target')
            if (source && target) {
              const sourceLabel = sankeyDatumValue(datum, 'sourceLabel')
              const targetLabel = sankeyDatumValue(datum, 'targetLabel')
              return `${sourceLabel ?? source} -> ${targetLabel ?? target}`
            }
            return `${sankeyDatumValue(datum, 'name') ?? sankeyDatumValue(datum, 'rawLabel') ?? ''}`
          },
        },
        content: tooltipMetricLines(valueFormatter, labels),
      },
    },
    background: { fill: 'transparent' },
    animation: false,
  }
}
