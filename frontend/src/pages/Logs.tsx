import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { Download, Eye, RotateCcw, Inbox } from 'lucide-react';
import { getLogs } from '../api/client';
import { Pagination } from '../components/Pagination';
import { ReplayModal } from '../components/ReplayModal';
import { SkeletonTable } from '../components/Skeleton';
import { EmptyState } from '../components/EmptyState';
import type { RequestLog, LogListResult } from '../types';

export function Logs() {
  const [data, setData] = useState<LogListResult | null>(null);
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState('');
  const [model, setModel] = useState('');
  const [replayId, setReplayId] = useState<string | null>(null);
  const [replayBody, setReplayBody] = useState('');
  const [loading, setLoading] = useState(true);

  const load = async () => {
    setLoading(true);
    const params: Record<string, string> = { page: String(page), limit: '50' };
    if (status) params.status = status;
    if (model) params.model = model;
    try { setData(await getLogs(params)); } catch {}
    setLoading(false);
  };

  useEffect(() => { load(); }, [page, status, model]);

  const fmtTime = (s: string) => new Date(s).toLocaleString();
  const handleReplay = (log: RequestLog) => {
    setReplayId(log.id);
    setReplayBody(log.downstream_req);
  };
  const protocolLabel = (log: RequestLog) => log.ingress_protocol || (log.downstream_path === '/v1/messages' ? 'anthropic' : 'openai');

  return (
    <div>
      <div className="page-header">
        <h2>请求日志</h2>
        <div style={{ display: 'flex', gap: 8 }}>
          <select className="input" value={status} onChange={e => { setStatus(e.target.value); setPage(1); }}>
            <option value="">全部状态</option>
            <option value="success">成功</option>
            <option value="error">失败</option>
          </select>
          <input className="input" placeholder="模型筛选" value={model} onChange={e => { setModel(e.target.value); setPage(1); }} />
          <a className="btn" href="/admin/logs/export?format=json" target="_blank" rel="noopener">
            <Download size={16} /> 导出
          </a>
        </div>
      </div>

      {loading ? (
        <div className="card">
          <SkeletonTable rows={6} cols={6} />
        </div>
      ) : !data || data.items.length === 0 ? (
        <div className="card">
          <EmptyState
            icon={Inbox}
            title="暂无请求日志"
            description="请求通过后将在此处显示记录。"
          />
        </div>
      ) : (
        <>
          <div className="table-scroll card" style={{ padding: 0 }}>
            <table>
              <thead>
                <tr>
                  <th>时间</th><th>模型</th><th>状态</th><th>TTFT</th><th>Token</th><th>操作</th>
                </tr>
              </thead>
              <tbody>
                {data?.items.map(log => (
                  <tr key={log.id}>
                    <td>{fmtTime(log.created_at)}</td>
                    <td>
                      <div>{log.model}{log.model !== log.mapped_model && ` → ${log.mapped_model}`}</div>
                      <div style={{ display: 'flex', gap: 6, marginTop: 4, flexWrap: 'wrap' }}>
                        <span className="badge">{protocolLabel(log)}</span>
                        {log.canonical_record && <span className="badge badge-success">canonical</span>}
                      </div>
                    </td>
                    <td><span className={`badge ${log.status === 'success' ? 'badge-success' : 'badge-error'}`}>{log.status}</span></td>
                    <td>{log.ttft_ms > 0 ? `${log.ttft_ms}ms` : '-'}</td>
                    <td>{log.total_tokens > 0 ? log.total_tokens.toLocaleString() : '-'}</td>
                    <td>
                      <Link to={`/logs/${log.id}`} className="btn" style={{ marginRight: 4 }}>
                        <Eye size={14} />
                      </Link>
                      <button className="btn" onClick={() => handleReplay(log)}>
                        <RotateCcw size={14} />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {data && <Pagination page={data.page} total={data.total} limit={data.limit} onChange={setPage} />}
        </>
      )}
      {replayId && <ReplayModal logId={replayId} originalBody={replayBody} onClose={() => setReplayId(null)} />}
    </div>
  );
}
