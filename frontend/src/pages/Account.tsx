import { useState, useEffect, useRef } from 'react';
import { LogIn, RefreshCw, X, Cpu, Globe } from 'lucide-react';
import { getAccount, refreshAccount, startBootstrap, getBootstrapStatus, cancelBootstrap } from '../api/client';
import { StatCard } from '../components/StatCard';
import { Skeleton } from '../components/Skeleton';
import type { AccountData, BootstrapMethod, BootstrapResponse } from '../types';

export function Account() {
  const [data, setData] = useState<AccountData | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [bootstrap, setBootstrap] = useState<BootstrapResponse | null>(null);
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

  return (
    <div>
      <div className="page-header">
        <h2>账号管理</h2>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          <button
            className="btn btn-primary"
            onClick={() => handleBootstrap('oauth')}
            disabled={inFlight}
            title="使用 OAuth 浏览器登录，需要 config.yaml 配置 lingma.client_id"
          >
            <LogIn size={16} />
            OAuth 登录
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
          <button
            className="btn"
            onClick={() => handleBootstrap('remote_callback')}
            disabled={inFlight}
            title="启动一次性 127.0.0.1:37510 回调，浏览器登录后自动写入凭据，无需 client_id 也无需本地灵码"
          >
            <Globe size={16} />
            远程登录
          </button>
          <button className="btn" onClick={handleRefresh} disabled={refreshing}>
            <RefreshCw size={16} />
            {refreshing ? '刷新中...' : '刷新凭据'}
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
                style={{ wordBreak: 'break-all', color: 'var(--primary)', fontWeight: 600 }}>
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
              {bootstrap.method === 'oauth' && bootstrap.error?.includes('client_id') && (
                <span> 请先在 config.yaml 的 lingma 区块配置 client_id，或改用「远程登录」/「本地灵码」。</span>
              )}
              {bootstrap.method === 'ws' && bootstrap.error?.toLowerCase().includes('websocket') && (
                <span> 请确保本机灵码客户端正在运行（端口 37010），或改用「OAuth 登录」/「远程登录」。</span>
              )}
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
            <tr><td style={{ fontWeight: 600 }}>CosyKey</td><td>{data.credential?.cos_y_key ? mask(data.credential.cos_y_key) : '-'}</td></tr>
            <tr><td style={{ fontWeight: 600 }}>EncryptUserInfo</td><td>{data.credential?.encrypt_user_info ? mask(data.credential.encrypt_user_info) : '-'}</td></tr>
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
            <tr><td style={{ fontWeight: 600 }}>加载时间</td><td>{data.status?.loaded_at ? new Date(data.status.loaded_at).toLocaleString() : '-'}</td></tr>
            <tr><td style={{ fontWeight: 600 }}>来源</td><td>{data.status?.source || '-'}</td></tr>
          </tbody>
        </table>
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
