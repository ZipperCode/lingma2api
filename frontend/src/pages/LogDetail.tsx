import { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft, Copy, RotateCcw } from 'lucide-react';
import { getLog } from '../api/client';
import { CodeViewer } from '../components/CodeViewer';
import { ReplayModal } from '../components/ReplayModal';
import { Skeleton } from '../components/Skeleton';
import type { RequestLog } from '../types';

const BASE_TABS = [
  { key: 'downstream_req', label: '下游请求' },
  { key: 'upstream_req', label: '上游请求' },
  { key: 'upstream_resp', label: '上游响应' },
  { key: 'downstream_resp', label: '下游响应' },
] as const;

const CANONICAL_TABS = [
  { key: 'pre_policy_request', label: 'Pre-Policy Canonical' },
  { key: 'post_policy_request', label: 'Post-Policy Canonical' },
  { key: 'session_snapshot', label: 'Canonical Session' },
] as const;

export function LogDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [log, setLog] = useState<RequestLog | null>(null);
  const [tab, setTab] = useState('downstream_req');
  const [showReplay, setShowReplay] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (id) {
      setLoading(true);
      getLog(id).then(d => { setLog(d); setLoading(false); }).catch(() => navigate('/logs'));
    }
  }, [id, navigate]);

  if (loading || !log) {
    return (
      <div>
        <div className="page-header">
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <button className="btn" onClick={() => navigate('/logs')}>
              <ArrowLeft size={16} /> 返回
            </button>
            <h2>请求详情</h2>
          </div>
        </div>
        <div className="card" style={{ marginBottom: 16, display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 16 }}>
          {Array.from({ length: 10 }).map((_, i) => (
            <div key={i}>
              <Skeleton variant="text" style={{ width: 60, marginBottom: 6 }} />
              <Skeleton variant="text-lg" style={{ width: 120 }} />
            </div>
          ))}
        </div>
        <Skeleton variant="rect" style={{ height: 400 }} />
      </div>
    );
  }

  const tabs = [
    ...BASE_TABS,
    ...(log.canonical_record ? CANONICAL_TABS.filter(t => Boolean(log[t.key as keyof RequestLog])) : []),
  ];
  const tabValue = String(log[tab as keyof RequestLog] || '');
  const copyToClipboard = (text: string) => navigator.clipboard.writeText(text);
  const protocolLabel = log.ingress_protocol || (log.downstream_path === '/v1/messages' ? 'anthropic' : 'openai');

  return (
    <div>
      <div className="page-header">
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <button className="btn" onClick={() => navigate('/logs')}>
            <ArrowLeft size={16} /> 返回
          </button>
          <h2>请求详情</h2>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button className="btn" onClick={() => copyToClipboard(tabValue)}>
            <Copy size={16} /> 复制
          </button>
          <button className="btn btn-primary" onClick={() => setShowReplay(true)}>
            <RotateCcw size={16} /> 重发
          </button>
        </div>
      </div>

      <div className="card" style={{ marginBottom: 16, display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 16 }}>
        <div><span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>时间</span><br />{new Date(log.created_at).toLocaleString()}</div>
        <div><span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>状态</span><br /><span className={`badge ${log.status === 'success' ? 'badge-success' : 'badge-error'}`}>{log.upstream_status}</span></div>
        <div><span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>模型</span><br />{log.model} → {log.mapped_model}</div>
        <div><span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>协议</span><br />{protocolLabel}</div>
        <div><span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>Session</span><br />{log.session_id || '-'}</div>
        <div><span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>入口</span><br />{log.ingress_endpoint || log.downstream_path}</div>
        <div><span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>TTFT</span><br />{log.ttft_ms}ms</div>
        <div><span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>上游耗时</span><br />{log.upstream_ms}ms</div>
        <div><span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>下游耗时</span><br />{log.downstream_ms}ms</div>
        <div><span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>Token</span><br />P:{log.prompt_tokens} C:{log.completion_tokens} T:{log.total_tokens}</div>
        {log.canonical_record && <div><span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>记录来源</span><br /><span className="badge badge-success">canonical execution record</span></div>}
      </div>

      <div className="tabs">
        {tabs.map(t => (
          <button key={t.key} className={`tab-btn ${tab === t.key ? 'active' : ''}`} onClick={() => setTab(t.key)}>
            {t.label}
          </button>
        ))}
      </div>

      <CodeViewer code={tabValue} />

      {showReplay && <ReplayModal logId={log.id} originalBody={log.downstream_req} onClose={() => setShowReplay(false)} />}
    </div>
  );
}
