import { useState } from 'react';
import { ChevronDown, ChevronRight, Globe, Monitor, AlertTriangle } from 'lucide-react';
import type { HTTPExchange } from '../types';

interface ExchangeCardProps {
  exchange: HTTPExchange;
}

export function ExchangeCard({ exchange }: ExchangeCardProps) {
  const [expanded, setExpanded] = useState(false);
  const [activeTab, setActiveTab] = useState<'headers' | 'body' | 'raw'>('body');

  const isUpstream = exchange.direction === 'upstream';
  const isRequest = exchange.phase === 'request';
  const isError = exchange.status_code ? exchange.status_code >= 400 : !!exchange.error;

  const Icon = isUpstream ? Globe : Monitor;

  const fmtTime = (ts: string) => {
    const d = new Date(ts);
    return d.toLocaleTimeString('zh-CN', { hour12: false }) + '.' + String(d.getMilliseconds()).padStart(3, '0');
  };

  const parseHeaders = (): [string, string][] => {
    try {
      const h = JSON.parse(exchange.headers || '{}');
      return Object.entries(h);
    } catch {
      return [];
    }
  };

  const formatBody = (body: string) => {
    try {
      return JSON.stringify(JSON.parse(body), null, 2);
    } catch {
      return body;
    }
  };

  const borderColor = isError ? 'var(--error)' : 'var(--c-indigo)';

  return (
    <div
      style={{
        border: '1px solid',
        borderColor: isError ? 'rgba(244, 63, 94, 0.4)' : 'var(--border)',
        borderRadius: 'var(--radius)',
        background: isError ? 'rgba(244, 63, 94, 0.04)' : 'var(--bg-card)',
        overflow: 'hidden',
      }}
    >
      {/* Header line */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          padding: '12px 16px',
          cursor: 'pointer',
          background: `linear-gradient(90deg, ${borderColor}10, transparent)`,
        }}
        onClick={() => setExpanded(!expanded)}
      >
        <span style={{ color: 'var(--text-secondary)' }}>
          {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
        </span>
        <Icon size={16} style={{ color: isUpstream ? 'var(--c-cyan)' : 'var(--c-violet)' }} />
        <span style={{ fontSize: 13, fontWeight: 600 }}>
          {isUpstream ? '上游' : '下游'} {isRequest ? '请求' : '响应'}
        </span>
        {!isRequest && (
          <>
            <span style={{
              padding: '2px 8px',
              borderRadius: 12,
              fontSize: 12,
              fontWeight: 700,
              background: isError ? 'rgba(244, 63, 94, 0.15)' : 'rgba(16, 185, 129, 0.12)',
              color: isError ? 'var(--error)' : 'var(--success)',
            }}>
              {exchange.error || `HTTP ${exchange.status_code}`}
            </span>
            {(exchange.duration_ms ?? 0) > 0 && (
              <span style={{ fontSize: 12, color: 'var(--text-secondary)', fontWeight: 600 }}>
                {exchange.duration_ms}ms
              </span>
            )}
          </>
        )}
        {isRequest && exchange.method && (
          <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--text-secondary)' }}>
            {exchange.method}
          </span>
        )}
        <span style={{ fontSize: 12, color: 'var(--text-secondary)', flex: 1 }}>
          {isRequest ? (exchange.path || exchange.url) : ''}
        </span>
        <span style={{ fontSize: 11, color: 'var(--text-secondary)', fontFamily: 'monospace' }}>
          {fmtTime(exchange.timestamp)}
        </span>
      </div>

      {/* Expanded content */}
      {expanded && (
        <div style={{ padding: '0 16px 16px' }}>
          {/* Tabs */}
          <div style={{ display: 'flex', gap: 0, borderBottom: '2px solid var(--border)', marginBottom: 12 }}>
            {['body', exchange.headers ? 'headers' : null, exchange.raw_stream ? 'raw' : null]
              .filter((t): t is string => t !== null)
              .map(tab => (
              <button
                key={tab}
                style={{
                  padding: '8px 16px',
                  border: 'none',
                  background: 'none',
                  cursor: 'pointer',
                  color: activeTab === tab ? 'var(--primary)' : 'var(--text-secondary)',
                  fontSize: 13,
                  fontWeight: 600,
                  borderBottom: activeTab === tab ? '3px solid var(--primary)' : '3px solid transparent',
                  marginBottom: -2,
                }}
                onClick={() => setActiveTab(tab as 'body' | 'headers' | 'raw')}
              >
                {tab === 'body' ? 'Body' : tab === 'headers' ? 'Headers' : '原始 SSE'}
              </button>
            ))}
          </div>

          {/* Content */}
          <div style={{
            background: '#0f111a',
            color: '#c9d1d9',
            padding: 16,
            borderRadius: 'var(--radius-sm)',
            fontFamily: "'JetBrains Mono', 'Fira Code', 'Consolas', monospace",
            fontSize: 12,
            maxHeight: 400,
            overflow: 'auto',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-all',
            lineHeight: 1.6,
          }}>
            {activeTab === 'body' && formatBody(exchange.body || '')}
            {activeTab === 'headers' && (
              <div>
                {parseHeaders().map(([k, v]) => (
                  <div key={k}>
                    <span style={{ color: '#7ee787' }}>{k}</span>
                    <span>: </span>
                    <span style={{ color: '#a5d6ff' }}>{v}</span>
                  </div>
                ))}
              </div>
            )}
            {activeTab === 'raw' && (exchange.raw_stream || '').split('\n').slice(0, 200).join('\n')}
          </div>
        </div>
      )}
    </div>
  );
}
