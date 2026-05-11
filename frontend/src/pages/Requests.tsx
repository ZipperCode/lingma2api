import { useCallback, useEffect, useMemo, useState } from 'react';
import { Copy, Download, Eye, RotateCcw, Search, X } from 'lucide-react';
import { getLog, getLogs } from '../api/client';
import { CodeViewer } from '../components/CodeViewer';
import { EmptyState } from '../components/EmptyState';
import { ReplayModal } from '../components/ReplayModal';
import { SkeletonTable } from '../components/Skeleton';
import type { RequestLog, LogListResult } from '../types';

type StatusFilter = 'all' | 'success' | 'error';
type DetailTab = 'request' | 'response' | 'canonical';

export function Requests() {
  const [data, setData] = useState<LogListResult | null>(null);
  const [query, setQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [loading, setLoading] = useState(true);
  const [selectedLog, setSelectedLog] = useState<RequestLog | null>(null);
  const [detailTab, setDetailTab] = useState<DetailTab>('request');
  const [replayId, setReplayId] = useState<string | null>(null);
  const [replayBody, setReplayBody] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setData(await getLogs({ page: '1', limit: '100' }));
    } catch {
      setData({ items: [], total: 0, page: 1, limit: 100 });
    }
    setLoading(false);
  }, []);

  useEffect(() => { load(); }, [load]);

  const filtered = useMemo(() => {
    const items = data?.items ?? [];
    const q = query.trim().toLowerCase();
    return items.filter((item) => {
      const matchesStatus = statusFilter === 'all' || item.status === statusFilter;
      const target = `${item.model} ${item.mapped_model} ${item.downstream_path} ${item.ingress_protocol || ''} ${item.upstream_status}`.toLowerCase();
      const matchesQuery = !q || target.includes(q);
      return matchesStatus && matchesQuery;
    });
  }, [data, query, statusFilter]);

  const handleSelect = useCallback(async (log: RequestLog) => {
    try {
      setSelectedLog(await getLog(log.id));
    } catch {
      setSelectedLog(log);
    }
    setDetailTab('request');
  }, []);

  const protocolLabel = (log: RequestLog) => log.ingress_protocol || (log.downstream_path === '/v1/messages' ? 'anthropic' : 'openai');

  const copyText = async (text: string) => {
    await navigator.clipboard.writeText(text);
  };

  const activeDetail = getDetailPayload(selectedLog, detailTab);

  return (
    <div className="requests-shell">
      <div className="page-header">
        <h2>请求流</h2>
        <div className="page-actions page-actions-wrap">
          <button className="btn" onClick={load}>刷新</button>
          <a className="btn" href="/admin/logs/export?format=json" target="_blank" rel="noopener">
            <Download size={16} /> 导出
          </a>
        </div>
      </div>

      <div className="requests-toolbar">
        <label className="search-field">
          <Search size={15} />
          <input
            className="input"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索模型、路径、协议或状态码"
          />
        </label>
        <div className="segment-control">
          {(['all', 'success', 'error'] as const).map((value) => (
            <button
              key={value}
              className={statusFilter === value ? 'active' : ''}
              onClick={() => setStatusFilter(value)}
            >
              {segmentLabel(value)}
            </button>
          ))}
        </div>
        <div className="page-meta-inline">Showing {filtered.length} of {data?.total ?? 0}</div>
      </div>

      <div className="requests-layout">
        <section className="card card-table requests-table-panel">
          {loading ? (
            <div className="requests-table-loading">
              <SkeletonTable rows={8} cols={6} />
            </div>
          ) : filtered.length === 0 ? (
            <EmptyState
              icon={Eye}
              title="暂无请求流数据"
              description="发起请求后，这里会展示最近的上下游调用轨迹。"
            />
          ) : (
            <div className="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>时间</th>
                    <th>路径</th>
                    <th>模型</th>
                    <th>状态</th>
                    <th>耗时</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((log) => (
                    <tr
                      key={log.id}
                      className={selectedLog?.id === log.id ? 'row-selected' : ''}
                      onClick={() => handleSelect(log)}
                      style={{ cursor: 'pointer' }}
                    >
                      <td>{new Date(log.created_at).toLocaleString()}</td>
                      <td>
                        <div className="cell-main">{log.downstream_path}</div>
                        <div className="cell-sub">
                          <span className="badge">{protocolLabel(log)}</span>
                          {log.canonical_record && <span className="badge badge-success">canonical</span>}
                        </div>
                      </td>
                      <td>
                        <div className="cell-main">{log.model || '-'}</div>
                        <div className="cell-sub">{log.mapped_model && log.mapped_model !== log.model ? `→ ${log.mapped_model}` : 'direct'}</div>
                      </td>
                      <td>
                        <span className={`badge ${log.status === 'success' ? 'badge-success' : 'badge-error'}`}>
                          {log.upstream_status || log.status}
                        </span>
                      </td>
                      <td>{formatLatency(log)}</td>
                      <td>
                        <div className="table-action-row">
                          <button className="btn" onClick={(event) => { event.stopPropagation(); handleSelect(log); }}>
                            <Eye size={14} />
                          </button>
                          <button className="btn" onClick={(event) => {
                            event.stopPropagation();
                            setReplayId(log.id);
                            setReplayBody(log.downstream_req);
                          }}>
                            <RotateCcw size={14} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <aside className="card requests-detail-panel">
          {selectedLog ? (
            <>
              <div className="detail-panel-header">
                <div>
                  <h3>请求详情</h3>
                  <p>{selectedLog.id.slice(0, 18)}...</p>
                </div>
                <button className="btn" onClick={() => setSelectedLog(null)}>
                  <X size={14} /> 关闭
                </button>
              </div>

              <div className="request-meta-grid">
                <div><span>协议</span><strong>{protocolLabel(selectedLog)}</strong></div>
                <div><span>模型</span><strong>{selectedLog.model || '-'}</strong></div>
                <div><span>映射后</span><strong>{selectedLog.mapped_model || '-'}</strong></div>
                <div><span>TTFT</span><strong>{selectedLog.ttft_ms > 0 ? `${selectedLog.ttft_ms}ms` : '-'}</strong></div>
              </div>

              <div className="segment-control detail-segment">
                <button className={detailTab === 'request' ? 'active' : ''} onClick={() => setDetailTab('request')}>请求</button>
                <button className={detailTab === 'response' ? 'active' : ''} onClick={() => setDetailTab('response')}>响应</button>
                <button
                  className={detailTab === 'canonical' ? 'active' : ''}
                  onClick={() => setDetailTab('canonical')}
                  disabled={!selectedLog.canonical_record}
                >
                  Canonical
                </button>
              </div>

              <div className="detail-toolbar-inline">
                <button className="btn" onClick={() => copyText(activeDetail)}>
                  <Copy size={14} /> 复制内容
                </button>
              </div>

              <CodeViewer code={activeDetail || '{}'} />
            </>
          ) : (
            <EmptyState
              icon={Eye}
              title="选择一条请求"
              description="右侧会展示请求体、响应体或 canonical 数据，并支持复制与重放。"
            />
          )}
        </aside>
      </div>

      {replayId && (
        <ReplayModal
          logId={replayId}
          originalBody={replayBody}
          onClose={() => setReplayId(null)}
        />
      )}
    </div>
  );
}

function formatLatency(log: RequestLog) {
  if (log.upstream_ms > 0) return `${log.upstream_ms}ms`;
  if (log.downstream_ms > 0) return `${log.downstream_ms}ms`;
  if (log.ttft_ms > 0) return `${log.ttft_ms}ms`;
  return '-';
}

function segmentLabel(value: StatusFilter) {
  switch (value) {
    case 'success':
      return '成功';
    case 'error':
      return '错误';
    default:
      return '全部';
  }
}

function getDetailPayload(log: RequestLog | null, tab: DetailTab) {
  if (!log) return '';
  if (tab === 'response') {
    return log.upstream_resp || log.downstream_resp || '{}';
  }
  if (tab === 'canonical') {
    return log.pre_policy_request || log.post_policy_request || log.session_snapshot || '{}';
  }
  return log.downstream_req || log.upstream_req || '{}';
}
