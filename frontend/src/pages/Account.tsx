import { useState, useEffect, useRef } from 'react';
import { RefreshCw, X, Globe, Zap, AlertTriangle, CheckCircle, ClipboardPaste } from 'lucide-react';
import { getAccount, refreshAccount, startBootstrap, getBootstrapStatus, cancelBootstrap, submitBootstrapCallback, testAccountConnection } from '../api/client';
import { StatCard } from '../components/StatCard';
import { Skeleton } from '../components/Skeleton';
import type { AccountData, AccountTestResult, BootstrapMethod, BootstrapResponse } from '../types';

const BOOTSTRAP_PHASES = [
  { key: 'awaiting_callback_url', label: '等待回填链接', icon: '1' },
  { key: 'parsing_callback', label: '解析回调', icon: '2' },
  { key: 'generating_cosy', label: '生成 COSY', icon: '3' },
  { key: 'deriving_remote', label: '远程派生', icon: '4' },
  { key: 'saving', label: '保存完成', icon: '5' },
];

function PhaseProgress({ phase, status }: { phase?: string; status: string }) {
  if (!phase) return null;
  const currentIdx = BOOTSTRAP_PHASES.findIndex(p => p.key === phase);
  return (
    <div style={{ display: 'flex', gap: 4, marginTop: 12, alignItems: 'center' }}>
      {BOOTSTRAP_PHASES.map((p, i) => {
        const active = currentIdx >= 0 && i <= currentIdx;
        const isCurrent = i === currentIdx;
        return (
          <div key={p.key} style={{
            flex: 1,
            padding: '8px 4px',
            borderRadius: 6,
            textAlign: 'center',
            fontSize: 12,
            fontWeight: isCurrent ? 700 : 400,
            background: active ? 'var(--primary-dim, rgba(59,130,246,0.1))' : 'var(--bg-secondary)',
            color: active ? 'var(--primary)' : 'var(--text-secondary)',
            border: isCurrent ? '1px solid var(--primary)' : '1px solid transparent',
            transition: 'all 0.2s',
          }}>
            <div style={{ fontSize: 16, marginBottom: 2 }}>{p.icon}</div>
            {p.label}
          </div>
        );
      })}
    </div>
  );
}

function Badge({ ok, label }: { ok: boolean; label: string }) {
  return (
    <span className={`badge ${ok ? 'badge-success' : 'badge-error'}`} style={{ fontSize: 12 }}>
      {ok ? label : `缺失 ${label}`}
    </span>
  );
}

function formatExpireTime(ms: string): string {
  if (!ms) return '-';
  const n = parseInt(ms, 10);
  if (n <= 0) return '-';
  const date = new Date(n);
  return date.toLocaleString();
}

function isExpired(ms: string): boolean {
  if (!ms) return false;
  const n = parseInt(ms, 10);
  if (n <= 0) return false;
  return Date.now() > n;
}

export function Account() {
  const [data, setData] = useState<AccountData | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [bootstrap, setBootstrap] = useState<BootstrapResponse | null>(null);
  const [callbackURL, setCallbackURL] = useState('');
  const [submittingCallback, setSubmittingCallback] = useState(false);
  const [testResult, setTestResult] = useState<AccountTestResult | null>(null);
  const [testing, setTesting] = useState(false);
  const pollRef = useRef<ReturnType<typeof setInterval>>();
  const [loading, setLoading] = useState(true);
  const [remaining, setRemaining] = useState<string>('');
  const tickRef = useRef<ReturnType<typeof setInterval>>();

  const formatRemaining = (expiresAt?: string): string => {
    if (!expiresAt) return '';
    const ms = new Date(expiresAt).getTime() - Date.now();
    if (ms <= 0) return '已超时';
    const total = Math.floor(ms / 1000);
    const m = Math.floor(total / 60);
    const s = total % 60;
    return `${m}:${s.toString().padStart(2, '0')}`;
  };

  const load = async () => {
    setLoading(true);
    try { setData(await getAccount()); } catch {}
    setLoading(false);
  };

  useEffect(() => { load(); }, []);

  useEffect(() => {
    return () => { if (pollRef.current) clearInterval(pollRef.current); };
  }, []);

  useEffect(() => {
    const status = bootstrap?.status;
    const inFlight = status === 'awaiting_callback_url' || status === 'running';
    if (inFlight) {
      setRemaining(formatRemaining(bootstrap?.expires_at));
      tickRef.current = setInterval(() => {
        setRemaining(formatRemaining(bootstrap?.expires_at));
      }, 1000);
      return () => {
        if (tickRef.current) clearInterval(tickRef.current);
      };
    }
    if (tickRef.current) {
      clearInterval(tickRef.current);
      tickRef.current = undefined;
    }
    setRemaining('');
    return undefined;
  }, [bootstrap?.status, bootstrap?.expires_at]);

  const handleRefresh = async () => {
    setRefreshing(true);
    try { await refreshAccount(); await load(); } catch {}
    setRefreshing(false);
  };

  const handleTest = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await testAccountConnection();
      setTestResult(result);
    } catch (e) {
      setTestResult({
        success: false,
        status_code: 0,
        response_preview: '',
        error: e instanceof Error ? e.message : String(e),
        credential_snapshot: {
          has_cosy_key: false,
          has_encrypt_user_info: false,
          has_user_id: false,
          has_machine_id: false,
          cosy_key_prefix: '',
          user_id: '',
        },
        timestamp: new Date().toISOString(),
      });
    }
    setTesting(false);
  };

  const startPolling = (id: string) => {
    if (pollRef.current) clearInterval(pollRef.current);
    pollRef.current = setInterval(async () => {
      try {
        const status = await getBootstrapStatus(id);
        setBootstrap(status);
        if (status.status === 'completed') {
          clearInterval(pollRef.current);
          await load();
        } else if (status.status === 'error' || status.status === 'cancelled') {
          clearInterval(pollRef.current);
        }
      } catch {
        // keep polling
      }
    }, 2000);
  };

  const handleBootstrap = async (method: BootstrapMethod) => {
    setCallbackURL('');
    try {
      const resp = await startBootstrap(method);
      setBootstrap(resp);
      startPolling(resp.id);
    } catch (e) {
      setBootstrap({
        id: '',
        status: 'error',
        method,
        error: e instanceof Error ? e.message : String(e),
        started_at: '',
      });
    }
  };

  const handleSubmitCallback = async () => {
    if (!bootstrap?.id || !callbackURL.trim()) return;
    setSubmittingCallback(true);
    try {
      const resp = await submitBootstrapCallback({ id: bootstrap.id, callback_url: callbackURL.trim() });
      setBootstrap(resp);
      if (resp.status === 'completed') {
        await load();
      } else {
        startPolling(resp.id);
      }
    } catch (e) {
      setBootstrap(prev => prev ? {
        ...prev,
        status: 'error',
        error: e instanceof Error ? e.message : String(e),
      } : {
        id: '',
        status: 'error',
        method: 'remote_callback',
        error: e instanceof Error ? e.message : String(e),
        started_at: '',
      });
    }
    setSubmittingCallback(false);
  };

  const handleCancel = async () => {
    if (!bootstrap?.id) return;
    try {
      await cancelBootstrap(bootstrap.id);
      if (pollRef.current) clearInterval(pollRef.current);
      setBootstrap({ ...bootstrap, status: 'cancelled', error: '', phase: '' });
    } catch (e) {
      setBootstrap({
        ...bootstrap,
        status: 'error',
        error: e instanceof Error ? e.message : String(e),
      });
    }
  };

  const mask = (s: string) => s.length > 6 ? s.slice(0, 3) + '***' + s.slice(-3) : s;
  const fmtToken = (n: number) => n >= 1000000 ? `${(n / 1000000).toFixed(1)}M` : n >= 1000 ? `${(n / 1000).toFixed(1)}K` : String(n);
  const inFlight =
    bootstrap?.status === 'awaiting_callback_url' ||
    bootstrap?.status === 'running';

  if (loading || !data) {
    return (
      <div>
        <div className="page-header"><h2>账号管理</h2></div>
        <Skeleton variant="rect" style={{ height: 120, marginBottom: 16 }} />
        <Skeleton variant="rect" style={{ height: 160, marginBottom: 16 }} />
        <Skeleton variant="rect" style={{ height: 120 }} />
      </div>
    );
  }

  const hasCosy = data.credential?.cosy_key !== '';
  const hasEUI = data.credential?.encrypt_user_info !== '';
  const tokenExpired = isExpired(data.stored_meta?.token_expire_time || '');

  return (
    <div>
      <div className="page-header">
        <h2>账号管理</h2>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          <button
            className="btn btn-primary"
            onClick={() => handleBootstrap('remote_callback')}
            disabled={inFlight}
            title="生成浏览器登录链接，完成登录后把 127.0.0.1:37510 的最终回调地址粘贴回此页面"
          >
            <Globe size={16} />
            浏览器登录
          </button>
          <button className="btn" onClick={handleRefresh} disabled={refreshing}>
            <RefreshCw size={16} />
            {refreshing ? '读取中...' : '重新读取凭据'}
          </button>
          <button className="btn" onClick={handleTest} disabled={testing}>
            <Zap size={16} />
            {testing ? '测试中...' : '测试连接'}
          </button>
        </div>
      </div>

      {bootstrap && (
        <div className="card" style={{ marginBottom: 16, borderLeft: '3px solid var(--primary)' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
            <h4 style={{ margin: 0 }}>
              {bootstrap.status === 'awaiting_callback_url' && '等待回填回调链接'}
              {bootstrap.status === 'running' && '正在处理回调链接'}
              {bootstrap.status === 'completed' && '登录完成'}
              {bootstrap.status === 'error' && '登录失败'}
              {bootstrap.status === 'cancelled' && '已取消'}
            </h4>
            {inFlight && (
              <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                {remaining && <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>剩余 {remaining}</span>}
                <button className="btn btn-sm" onClick={handleCancel}>
                  <X size={14} /> 取消
                </button>
              </div>
            )}
          </div>
          {bootstrap.auth_url && bootstrap.status === 'awaiting_callback_url' && (
            <div style={{ marginBottom: 8 }}>
              <p style={{ marginBottom: 8 }}>请在你自己的浏览器中打开以下链接完成阿里云登录：</p>
              <a href={bootstrap.auth_url} target="_blank" rel="noopener noreferrer"
                style={{ wordBreak: 'break-all', color: 'var(--primary)', fontWeight: 600, fontSize: 13 }}>
                {bootstrap.auth_url}
              </a>
              <div style={{ marginTop: 12, padding: 12, borderRadius: 8, background: 'var(--bg-secondary)' }}>
                <p style={{ margin: 0, fontSize: 13, color: 'var(--text-secondary)' }}>
                  登录完成后，浏览器会跳到 `127.0.0.1:37510`。即使页面打不开，也请直接复制地址栏里的完整回调链接并粘贴到下面。
                </p>
              </div>
              <div style={{ display: 'grid', gap: 8, marginTop: 12 }}>
                <textarea
                  value={callbackURL}
                  onChange={(e) => setCallbackURL(e.target.value)}
                  placeholder="粘贴完整的 http://127.0.0.1:37510/... 回调链接"
                  rows={4}
                  style={{ width: '100%', resize: 'vertical' }}
                />
                <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                  <button className="btn btn-primary" onClick={handleSubmitCallback} disabled={submittingCallback || !callbackURL.trim()}>
                    <ClipboardPaste size={16} />
                    {submittingCallback ? '提交中...' : '提交回调链接'}
                  </button>
                </div>
              </div>
            </div>
          )}
          {bootstrap.status === 'running' && (
            <p style={{ color: 'var(--text-secondary)' }}>
              正在解析回调参数并生成凭据，请稍候。
            </p>
          )}
          <PhaseProgress phase={bootstrap.phase || bootstrap.status} status={bootstrap.status} />
          {bootstrap.status === 'completed' && (
            <p style={{ color: 'var(--success)' }}>凭据已成功更新。</p>
          )}
          {bootstrap.status === 'cancelled' && (
            <p style={{ color: 'var(--text-secondary)' }}>已取消登录流程，未写入任何凭据。</p>
          )}
          {bootstrap.status === 'error' && (
            <p style={{ color: 'var(--error)' }}>
              {bootstrap.error || '未知错误'}
              {bootstrap.error?.includes('timeout') && (
                <span> 5 分钟内未完成登录或未回填回调链接，请重试。</span>
              )}
              {bootstrap.error?.includes('callback url host must be') && (
                <span> 请确认粘贴的是完整的 `127.0.0.1:37510` 回调地址。</span>
              )}
              {bootstrap.error?.includes('all remote login strategies failed') && (
                <span> 远程派生 cosy_key 失败，请稍后重试。</span>
              )}
            </p>
          )}
        </div>
      )}

      {testResult && (
        <div className="card" style={{ marginBottom: 16, borderLeft: testResult.success ? '3px solid var(--success)' : '3px solid var(--error)' }}>
          <h4 style={{ marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
            {testResult.success ? <CheckCircle size={18} color="var(--success)" /> : <AlertTriangle size={18} color="var(--error)" />}
            API 连接测试
          </h4>
          <table>
            <tbody>
              <tr><td style={{ fontWeight: 600 }}>状态</td><td>
                <span className={`badge ${testResult.success ? 'badge-success' : 'badge-error'}`}>
                  {testResult.success ? '成功' : '失败'}
                </span>
              </td></tr>
              <tr><td style={{ fontWeight: 600 }}>HTTP 状态码</td><td>{testResult.status_code || '-'}</td></tr>
              <tr><td style={{ fontWeight: 600 }}>响应预览</td><td>{testResult.response_preview || '-'}</td></tr>
              {testResult.error && <tr><td style={{ fontWeight: 600 }}>错误信息</td><td style={{ color: 'var(--error)' }}>{testResult.error}</td></tr>}
              <tr><td style={{ fontWeight: 600 }}>CosyKey</td><td><Badge ok={testResult.credential_snapshot.has_cosy_key} label="CosyKey" /></td></tr>
              <tr><td style={{ fontWeight: 600 }}>EncryptUserInfo</td><td><Badge ok={testResult.credential_snapshot.has_encrypt_user_info} label="EncryptUserInfo" /></td></tr>
              <tr><td style={{ fontWeight: 600 }}>UserID</td><td><Badge ok={testResult.credential_snapshot.has_user_id} label="UserID" /> {testResult.credential_snapshot.user_id}</td></tr>
              <tr><td style={{ fontWeight: 600 }}>MachineID</td><td><Badge ok={testResult.credential_snapshot.has_machine_id} label="MachineID" /></td></tr>
              <tr><td style={{ fontWeight: 600 }}>CosyKey 前缀</td><td>{testResult.credential_snapshot.cosy_key_prefix || '-'}</td></tr>
            </tbody>
          </table>
        </div>
      )}

      <div className="card" style={{ marginBottom: 16 }}>
        <h4 style={{ marginBottom: 12 }}>用户信息</h4>
        <table>
          <tbody>
            <tr><td style={{ fontWeight: 600 }}>UserID</td><td>{data.credential?.user_id || '-'}</td></tr>
            <tr><td style={{ fontWeight: 600 }}>MachineID</td><td>{data.credential?.machine_id || '-'}</td></tr>
            <tr><td style={{ fontWeight: 600 }}>CosyKey</td><td>
              <Badge ok={hasCosy} label="CosyKey" />
              {hasCosy && <span style={{ marginLeft: 8 }}>{mask(data.credential?.cosy_key || '')}</span>}
            </td></tr>
            <tr><td style={{ fontWeight: 600 }}>EncryptUserInfo</td><td>
              <Badge ok={hasEUI} label="EncryptUserInfo" />
              {hasEUI && <span style={{ marginLeft: 8 }}>{mask(data.credential?.encrypt_user_info || '')}</span>}
            </td></tr>
          </tbody>
        </table>
      </div>

      <div className="card" style={{ marginBottom: 16 }}>
        <h4 style={{ marginBottom: 12 }}>凭据状态</h4>
        <table>
          <tbody>
            <tr>
              <td style={{ fontWeight: 600 }}>状态</td>
              <td><span className={`badge ${data.status?.loaded ? 'badge-success' : 'badge-error'}`}>
                {data.status?.loaded ? '有效' : '无效'}
              </span></td>
            </tr>
            {tokenExpired && (
              <tr>
                <td style={{ fontWeight: 600 }}>Token 过期</td>
                <td><span className="badge badge-error"><AlertTriangle size={12} style={{ marginRight: 4 }} />已过期</span></td>
              </tr>
            )}
            <tr><td style={{ fontWeight: 600 }}>加载时间</td><td>{data.status?.loaded_at ? new Date(data.status.loaded_at).toLocaleString() : '-'}</td></tr>
            <tr><td style={{ fontWeight: 600 }}>来源</td><td>{data.status?.source || '-'}</td></tr>
            <tr><td style={{ fontWeight: 600 }}>Token 过期时间</td><td>{formatExpireTime(data.stored_meta?.token_expire_time || '')}</td></tr>
            <tr><td style={{ fontWeight: 600 }}>Lingma 版本</td><td>{data.stored_meta?.lingma_version_hint || '-'}</td></tr>
            <tr><td style={{ fontWeight: 600 }}>获取时间</td><td>{data.stored_meta?.obtained_at ? new Date(data.stored_meta.obtained_at).toLocaleString() : '-'}</td></tr>
            <tr><td style={{ fontWeight: 600 }}>更新时间</td><td>{data.stored_meta?.updated_at ? new Date(data.stored_meta.updated_at).toLocaleString() : '-'}</td></tr>
          </tbody>
        </table>
      </div>

      <div className="card" style={{ marginBottom: 16 }}>
        <h4 style={{ marginBottom: 12 }}>OAuth Token 状态</h4>
        <table>
          <tbody>
            <tr><td style={{ fontWeight: 600 }}>Access Token</td><td><Badge ok={data.oauth?.has_access_token || false} label="Access Token" /></td></tr>
            <tr><td style={{ fontWeight: 600 }}>Refresh Token</td><td><Badge ok={data.oauth?.has_refresh_token || false} label="Refresh Token" /></td></tr>
          </tbody>
        </table>
        {!data.oauth?.has_access_token && (
          <p style={{ marginTop: 8, color: 'var(--text-secondary)', fontSize: 13 }}>
            OAuth Token 缺失。请使用“浏览器登录”重新生成并回填回调链接。
          </p>
        )}
      </div>

      <div className="card">
        <h4 style={{ marginBottom: 12 }}>Token 用量统计</h4>
        <div className="stat-grid">
          <StatCard label="今日" value={fmtToken(data.token_stats?.today || 0)} />
          <StatCard label="本周" value={fmtToken(data.token_stats?.week || 0)} />
          <StatCard label="总计" value={fmtToken(data.token_stats?.total || 0)} />
        </div>
      </div>
    </div>
  );
}
