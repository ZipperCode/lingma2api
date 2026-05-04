import { useState } from 'react';
import { X, Clock, Bot, ArrowRight, Zap, Coins, CheckCircle, XCircle } from 'lucide-react';
import { ExchangeCard } from './ExchangeCard';
import type { RequestLog } from '../types';

interface LogDetailDrawerProps {
  log: RequestLog | null;
  onClose: () => void;
}

export function LogDetailDrawer({ log, onClose }: LogDetailDrawerProps) {
  if (!log) return null;

  const fmtFullTime = (s: string) => new Date(s).toLocaleString('zh-CN');

  const statusIcon = log.status === 'success' ? (
    <CheckCircle size={18} style={{ color: 'var(--success)' }} />
  ) : (
    <XCircle size={18} style={{ color: 'var(--error)' }} />
  );

  return (
    <>
      {/* Overlay */}
      <div
        style={{
          position: 'fixed',
          inset: 0,
          background: 'rgba(15, 23, 42, 0.4)',
          zIndex: 999,
        }}
        onClick={onClose}
      />
      {/* Drawer */}
      <div
        style={{
          position: 'fixed',
          top: 0,
          right: 0,
          bottom: 0,
          width: 'min(800px, 90vw)',
          background: 'var(--bg-solid)',
          borderLeft: '1px solid var(--border)',
          boxShadow: '-8px 0 32px rgba(0, 0, 0, 0.15)',
          zIndex: 1000,
          display: 'flex',
          flexDirection: 'column',
          animation: 'slideLeft 0.3s cubic-bezier(0.16, 1, 0.3, 1)',
        }}
      >
        {/* Drawer header */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '20px 24px',
            borderBottom: '1px solid var(--border)',
            background: 'var(--bg-card)',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <span style={{ fontSize: 16, fontWeight: 800 }}>请求详情</span>
            <span style={{ fontSize: 11, fontFamily: 'monospace', color: 'var(--text-secondary)' }}>
              {log.id.slice(0, 16)}...
            </span>
          </div>
          <button
            onClick={onClose}
            style={{
              border: 'none',
              background: 'none',
              cursor: 'pointer',
              color: 'var(--text-secondary)',
              padding: 4,
            }}
          >
            <X size={20} />
          </button>
        </div>

        {/* Content */}
        <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
          {/* Metadata grid */}
          <div style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))',
            gap: 16,
            marginBottom: 24,
          }}>
            <MetaItem icon={<Clock size={16} />} label="时间" value={fmtFullTime(log.created_at)} />
            <MetaItem icon={<Bot size={16} />} label="模型" value={`${log.model}${log.model !== log.mapped_model ? ` → ${log.mapped_model}` : ''}`} />
            <MetaItem icon={statusIcon} label="状态" value={log.status} />
            <MetaItem icon={<Zap size={16} />} label="TTFT" value={log.ttft_ms > 0 ? `${log.ttft_ms}ms` : '-'} />
            <MetaItem icon={<Coins size={16} />} label="Tokens" value={log.total_tokens > 0 ? `P:${log.prompt_tokens} C:${log.completion_tokens} T:${log.total_tokens}` : '-'} />
            <MetaItem label="协议" value={`${log.ingress_protocol || '-'} ${log.ingress_endpoint || ''}`} />
          </div>

          {/* Timeline */}
          <h4 style={{ fontSize: 14, fontWeight: 700, marginBottom: 16, color: 'var(--text)' }}>
            请求链路时间线
          </h4>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {log.exchanges && log.exchanges.length > 0 ? (
              log.exchanges.map(ex => <ExchangeCard key={ex.id} exchange={ex} />)
            ) : (
              <div style={{
                padding: 24,
                textAlign: 'center',
                color: 'var(--text-secondary)',
                fontSize: 14,
                border: '1px dashed var(--border)',
                borderRadius: 'var(--radius)',
              }}>
                无 HTTP 交换记录（旧日志数据）
              </div>
            )}
          </div>

          {/* Canonical data collapsible */}
          {log.pre_policy_request && (
            <details style={{ marginTop: 24 }}>
              <summary style={{
                cursor: 'pointer',
                fontSize: 14,
                fontWeight: 700,
                padding: '12px 0',
                color: 'var(--text)',
                borderTop: '1px solid var(--border)',
              }}>
                Canonical Data (Pre/Post Policy, Session)
              </summary>
              <div style={{ marginTop: 12, display: 'flex', flexDirection: 'column', gap: 16 }}>
                {log.pre_policy_request && <CodeBlock title="Pre-Policy Request" data={log.pre_policy_request} />}
                {log.post_policy_request && <CodeBlock title="Post-Policy Request" data={log.post_policy_request} />}
                {log.session_snapshot && <CodeBlock title="Session Snapshot" data={log.session_snapshot} />}
              </div>
            </details>
          )}
        </div>
      </div>
    </>
  );
}

function MetaItem({ icon, label, value }: { icon?: React.ReactNode; label: string; value: string }) {
  return (
    <div style={{
      padding: 12,
      background: 'var(--bg-card)',
      borderRadius: 'var(--radius-sm)',
      border: '1px solid var(--border)',
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
        {icon}
        <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
          {label}
        </span>
      </div>
      <div style={{ fontSize: 13, fontWeight: 600, wordBreak: 'break-all' }}>{value}</div>
    </div>
  );
}

function CodeBlock({ title, data }: { title: string; data: string }) {
  const [open, setOpen] = useState(false);
  return (
    <details open={open} onToggle={e => setOpen((e.target as HTMLDetailsElement).open)}>
      <summary style={{ cursor: 'pointer', fontSize: 13, fontWeight: 600, marginBottom: 8, color: 'var(--text-secondary)' }}>
        {title}
      </summary>
      <pre style={{
        background: '#0f111a',
        color: '#c9d1d9',
        padding: 16,
        borderRadius: 'var(--radius-sm)',
        fontSize: 12,
        overflow: 'auto',
        maxHeight: 300,
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-all',
        fontFamily: "'JetBrains Mono', 'Fira Code', 'Consolas', monospace",
        lineHeight: 1.6,
      }}>
        {data}
      </pre>
    </details>
  );
}
