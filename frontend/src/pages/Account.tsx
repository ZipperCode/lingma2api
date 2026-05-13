import { useState, useEffect, useRef } from 'react';
import { RefreshCw, X, Cpu, Globe, Download, Zap, AlertTriangle, CheckCircle } from 'lucide-react';
import { getAccount, refreshAccount, startBootstrap, getBootstrapStatus, cancelBootstrap, importCache, testAccountConnection } from '../api/client';
import { StatCard } from '../components/StatCard';
import { Skeleton } from '../components/Skeleton';
import type { AccountData, AccountTestResult, BootstrapMethod, BootstrapResponse } from '../types';

const BOOTSTRAP_PHASES = [
  { key: 'waiting_callback', label: '等待浏览器认证', icon: '1' },
  { key: 'parsing_credentials', label: '解码凭据', icon: '2' },
  { key: 'generating_cosy', label: '生成 COSY 密钥', icon: '3' },
  { key: 'saving', label: '保存完成', icon: '4' },
  { key: 'parsing_page_capture', label: '解析页面捕获', icon: '2b' },
  { key: 'deriving_remote', label: '远程派生凭据', icon: '3b' },
];

function PhaseProgress({ phase, status }: { phase?: string; status: string }) {
  if (status !== 'running' || !phase) return null;
  const currentIdx = BOOTSTRAP_PHASES.findIndex(p => p.key === phase);
  return (
    <div style={{ display: 'flex', gap: 4, marginTop: 12, alignItems: 'center' }}>
      {BOOTSTRAP_PHASES.filter(p => !p.key.endsWith('b')).map((p, i) => {
        const active = i === currentIdx || (i < currentIdx);
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
  const [importing, setImporting] = useState(false);
  const [importResult, setImportResult] = useState<{ status: string; user_id: string } | null>(null);
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
    const inFlight = status === 'running' || status === 'awaiting_callback' || status === 'deriving';
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

  const handleImportCache = async () => {
    setImporting(true);
    setImportResult(null);
    try {
      const result = await importCache();
      setImportResult({ status: result.status, user_id: result.user_id });
      await load();
    } catch (e) {
      setImportResult({ status: 'error', user_id: e instanceof Error ? e.message : String(e) });
    }
    setImporting(false);
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

  const handleBootstrap = async (method: BootstrapMethod) => {
    try {
      const resp = await startBootstrap(method);
      setBootstrap(resp);

      if (resp.status === 'completed') {
        await load();
        return;
      }

      pollRef.current = setInterval(async () => {
        try {
          const status = await getBootstrapStatus(resp.id);
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

  const handleCancel = async () => {
    if (!bootstrap?.id) return;
    try {
      await cancelBootstrap(bootstrap.id);
      if (pollRef.current) clearInterval(pollRef.current);
      setBootstrap({ ...bootstrap, status: 'cancelled', error: '' });
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
    bootstrap?.status === 'running' ||
    bootstrap?.status === 'awaiting_callback' ||
    bootstrap?.status === 'deriving';

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
  const hasUID = data.credential?.user_id !== '';
  const hasMID = data.credential?.machine_id !== '';
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
            title="启动一次性 127.0.0.1:37510 回调，浏览器登录后自动写入凭据，无需本地灵码客户端"
          >
            <Globe size={16} />
            浏览器登录
          </button>
          <button
            className="btn"
            onClick={() => handleBootstrap('ws')}
            disabled={inFlight}
            title="连接本机灵码客户端（监听 127.0.0.1:37010）派生凭据"
          >
            <Cpu size={16} />
            本地灵码
          </button>
          <button className="btn" onClick={handleImportCache} disabled={importing}>
            <Download size={16} />
            {importing ? '导入中...' : '导入本地缓存'}
          </button>
          <button className="btn" onClick={handleRefresh} disabled={refreshing}>
            <RefreshCw size={16} />
            {refreshing ? '刷新中...' : '刷新凭据'}
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
              {bootstrap.status === 'running' && '登录初始化中...'}
              {bootstrap.status === 'awaiting_callback' && '等待浏览器回调'}
              {bootstrap.status === 'deriving' && '派生凭据中...'}
              {bootstrap.status === 'completed' && '登录完成'}
              {bootstrap.status === 'error' && '登录失败'}
              {bootstrap.status === 'cancelled' && '已取消'}
            </h4>
            {(bootstrap.status === 'running' || bootstrap.status === 'awaiting_callback' || bootstrap.status === 'deriving') && (
              <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                {remaining && <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>剩余 {remaining}</span>}
                <button className="btn btn-sm" onClick={handleCancel}>
                  <X size={14} /> 取消
                </button>
              </div>
            )}
          </div>
          {bootstrap.auth_url && (bootstrap.status === 'running' || bootstrap.status === 'awaiting_callback') && (
            <div style={{ marginBottom: 8 }}>
              <p style={{ marginBottom: 8 }}>请在浏览器中打开以下链接完成阿里云登录：</p>
              <a href={bootstrap.auth_url} target="_blank" rel="noopener noreferrer"
                style={{ wordBreak: 'break-all', color: 'var(--primary)', fontWeight: 600, fontSize: 13 }}>
                {bootstrap.auth_url}
              </a>
              <p style={{ marginTop: 8, fontSize: 13, color: 'var(--text-secondary)' }}>
                登录完成后凭据将自动注入。本页面会在凭据写入后自动刷新。
              </p>
            </div>
          )}
          {bootstrap.status === 'deriving' && (
            <p style={{ color: 'var(--text-secondary)' }}>
              正在通过远程 user/login 派生 cosy_key 与 encrypt_user_info...
            </p>
          )}
          <PhaseProgress phase={bootstrap.phase} status={bootstrap.status} />
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
                <span> 5 分钟内未完成浏览器登录，请重试。</span>
              )}
              {bootstrap.error?.includes('all remote login strategies failed') && (
                <span> 远程派生 cosy_key 失败（可能被 WAF 拦截），请稍后重试或联系管理员。</span>
              )}
              {bootstrap.method === 'ws' && bootstrap.error?.toLowerCase().includes('websocket') && (
                <span> 请确保本机灵码客户端正在运行（端口 37010），或改用「浏览器登录」。</span>
              )}
            </p>
          )}
        </div>
      )}

      {importResult && (
        <div className="card" style={{ marginBottom: 16, borderLeft: importResult.status === 'imported' ? '3px solid var(--success)' : '3px solid var(--error)' }}>
          <p style={{ color: importResult.status === 'imported' ? 'var(--success)' : 'var(--error)' }}>
            {importResult.status === 'imported'
              ? `成功导入凭据 (UserID: ${importResult.user_id})`
              : importResult.user_id}
          </p>
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
          {!testResult.success && testResult.error?.includes('credentials unavailable') && (
            <p style={{ marginTop: 8, color: 'var(--text-secondary)', fontSize: 13 }}>
              凭据不完整，请检查是否所有字段都已正确注入。建议使用「浏览器登录」重新获取完整凭据。
            </p>
          )}
          {!testResult.success && (testResult.status_code === 401 || testResult.status_code === 403) && (
            <p style={{ marginTop: 8, color: 'var(--text-secondary)', fontSize: 13 }}>
              Token 认证失败，可能已过期或签名不匹配。请尝试「刷新凭据」或重新登录。
            </p>
          )}
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
            OAuth Token 缺失，自动刷新功能不可用。建议使用「浏览器登录」获取完整凭据。
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