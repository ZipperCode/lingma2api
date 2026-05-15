import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  LineChart,
  Line,
  BarChart,
  Bar,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts';
import {
  Activity,
  ArrowRight,
  Bot,
  Clock,
  Gauge,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  Trash2,
} from 'lucide-react';
import { cleanupLogs, getDashboard, getOverview, refreshAccount, refreshAdminModels } from '../api/client';
import { usePolling } from '../hooks/usePolling';
import { useSettings } from '../hooks/useSettings';
import { Skeleton } from '../components/Skeleton';
import { StatCard } from '../components/StatCard';
import type { DashboardData, OverviewData, RequestLog } from '../types';

const RANGES = ['1h', '24h', '7d', '30d'];
const COLORS = ['#cc785c', '#5db8a6', '#e8a55a', '#a9583e', '#5db872', '#c64545', '#8e8b82', '#252320'];

export function Dashboard() {
  const navigate = useNavigate();
  const { settings } = useSettings();
  const [range, setRange] = useState('24h');
  const [overview, setOverview] = useState<OverviewData | null>(null);
  const [chartData, setChartData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [busyAction, setBusyAction] = useState<'models' | 'credentials' | 'cleanup' | null>(null);
  const [notice, setNotice] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [overviewResult, dashboardResult] = await Promise.all([
        getOverview(),
        getDashboard(range),
      ]);
      setOverview(overviewResult);
      setChartData(dashboardResult);
    } finally {
      setLoading(false);
    }
  }, [range]);

  const pollInterval = parseInt(settings.polling_interval || '0', 10);
  usePolling(load, pollInterval);
  useEffect(() => { load(); }, [load]);

  const fmtToken = (n: number) => n >= 1000000 ? `${(n / 1000000).toFixed(1)}M` : n >= 1000 ? `${(n / 1000).toFixed(1)}K` : String(n);

  const requestBars = useMemo(() => {
    const items = overview?.recent_requests ?? [];
    if (items.length === 0) return [];
    const values = items
      .map((item) => requestLatency(item))
      .filter((value) => value > 0)
      .slice(0, 36)
      .reverse();
    if (values.length === 0) return [];
    const max = Math.max(...values);
    return values.map((value, index) => ({
      height: Math.max(16, Math.round((value / max) * 100)),
      opacity: 0.52 + index / 45,
    }));
  }, [overview]);

  const runAction = async (kind: 'models' | 'credentials' | 'cleanup', action: () => Promise<void>, successMessage: string) => {
    setBusyAction(kind);
    setNotice('');
    try {
      await action();
      setNotice(successMessage);
      await load();
    } catch (error) {
      setNotice(error instanceof Error ? error.message : String(error));
    }
    setBusyAction(null);
  };

  if (loading || !overview || !chartData) {
    return (
      <div>
        <div className="dashboard-status-strip">
          {Array.from({ length: 5 }).map((_, index) => (
            <Skeleton key={index} variant="rect" style={{ height: 92 }} />
          ))}
        </div>
        <div className="dashboard-shell-grid">
          <Skeleton variant="rect" style={{ height: 320 }} />
          <Skeleton variant="rect" style={{ height: 320 }} />
          <Skeleton variant="rect" style={{ height: 320 }} />
          <Skeleton variant="rect" style={{ height: 320 }} />
        </div>
      </div>
    );
  }

  return (
    <div>
      <div className="page-header">
        <h2>仪表盘</h2>
        <div className="page-actions page-actions-wrap">
          <select className="input" value={range} onChange={(event) => setRange(event.target.value)}>
            {RANGES.map((item) => <option key={item} value={item}>{item}</option>)}
          </select>
          <button className="btn" onClick={load}>
            <RefreshCw size={16} /> 刷新总览
          </button>
          {notice && <span className="page-meta-inline">{notice}</span>}
        </div>
      </div>

      <section className="dashboard-status-strip">
        <div className="card dashboard-strip-card">
          <span className={`status-chip ${overview.healthy ? 'ok' : 'err'}`}>{overview.healthy ? 'Healthy' : 'Attention'}</span>
          <strong>服务状态</strong>
          <span>{overview.healthy ? '凭据与模型缓存已就绪' : '请检查凭据或模型缓存'}</span>
        </div>
        <div className="card dashboard-strip-card">
          <ShieldCheck size={18} />
          <strong>凭据</strong>
          <span>{overview.credential.loaded ? '已加载' : '未加载'} · {overview.credential.source || 'unknown'}</span>
        </div>
        <div className="card dashboard-strip-card">
          <Bot size={18} />
          <strong>模型缓存</strong>
          <span>{overview.models.count} models · {overview.models.cached ? 'cached' : 'not ready'}</span>
        </div>
        <div className="card dashboard-strip-card">
          <Activity size={18} />
          <strong>会话</strong>
          <span>{overview.session_count} active sessions</span>
        </div>
        <div className="card dashboard-strip-card">
          <Clock size={18} />
          <strong>更新时间</strong>
          <span>{new Date(overview.generated_at).toLocaleString()}</span>
        </div>
      </section>

      <div className="stat-grid">
        <StatCard label="总请求数" value={chartData.stats.total_requests.toLocaleString()} icon={Activity} />
        <StatCard label="成功率" value={chartData.stats.success_rate.toFixed(1)} suffix="%" icon={Gauge} />
        <StatCard label="平均延迟" value={overview.latency.avg_ms || chartData.stats.avg_ttft_ms} suffix="ms" icon={Clock} />
        <StatCard label="Token 消耗" value={fmtToken(overview.token_stats.total || chartData.stats.total_tokens)} icon={Sparkles} />
      </div>

      <div className="dashboard-shell-grid">
        <section className="card dashboard-panel">
          <div className="card-header-inline">
            <div>
              <h4>Health</h4>
              <p className="section-subtle">最近请求窗口的延迟分布与系统响应状态。</p>
            </div>
            <span className={`status-chip ${overview.healthy ? 'ok' : 'warn'}`}>{overview.latency.sample_count} samples</span>
          </div>
          <div className="activity-chart" aria-label="Latency trend">
            {requestBars.length > 0 ? requestBars.map((bar, index) => (
              <span key={index} className="activity-bar" style={{ height: `${bar.height}%`, opacity: bar.opacity }} />
            )) : <span className="chart-empty">暂无请求延迟样本</span>}
          </div>
          <div className="health-stats-grid">
            <div><strong>{overview.latency.avg_ms || 0}</strong><span>Avg</span></div>
            <div><strong>{overview.latency.p50_ms || 0}</strong><span>P50</span></div>
            <div><strong>{overview.latency.p95_ms || 0}</strong><span>P95</span></div>
            <div><strong>{overview.latency.max_ms || 0}</strong><span>Max</span></div>
          </div>
        </section>

        <section className="card dashboard-panel">
          <div className="card-header-inline">
            <div>
              <h4>Models</h4>
              <p className="section-subtle">展示当前可用模型和快速刷新入口。</p>
            </div>
            <button
              className="btn"
              onClick={() => runAction('models', async () => { await refreshAdminModels(); }, '模型缓存已刷新')}
              disabled={busyAction === 'models'}
            >
              <RefreshCw size={14} /> {busyAction === 'models' ? '刷新中...' : '刷新模型'}
            </button>
          </div>
          <div className="model-stack">
            {overview.available_models.slice(0, 6).map((item) => (
              <button key={item.id} className="model-row-card" onClick={() => navigator.clipboard.writeText(item.id)}>
                <div>
                  <div className="cell-main">{item.id}</div>
                  <div className="cell-sub">{item.owned_by === 'lingma' ? 'upstream' : 'alias'}</div>
                </div>
                <span className={`status-chip ${item.owned_by === 'lingma' ? 'ok' : 'warn'}`}>{item.owned_by}</span>
              </button>
            ))}
          </div>
          <button className="link-button-inline" onClick={() => navigate('/models')}>
            打开完整模型页 <ArrowRight size={14} />
          </button>
        </section>

        <section className="card dashboard-panel">
          <div className="card-header-inline">
            <div>
              <h4>Configuration</h4>
              <p className="section-subtle">首页只展示关键运行配置，完整项在设置页维护。</p>
            </div>
          </div>
          <div className="config-summary-grid">
            <div><span>Storage</span><strong>{overview.settings.storage_mode || 'full'}</strong></div>
            <div><span>Retention</span><strong>{overview.settings.retention_days || '30'} days</strong></div>
            <div><span>Timeout</span><strong>{overview.settings.request_timeout || '90'}s</strong></div>
            <div><span>Polling</span><strong>{overview.settings.polling_interval || '0'}s</strong></div>
          </div>
          <div className="dashboard-quick-actions">
            <button
              className="btn"
              onClick={() => runAction('credentials', async () => { await refreshAccount(); }, '凭据已刷新')}
              disabled={busyAction === 'credentials'}
            >
              <ShieldCheck size={14} /> {busyAction === 'credentials' ? '刷新中...' : '刷新凭据'}
            </button>
            <button
              className="btn btn-danger"
              onClick={() => runAction('cleanup', async () => { await cleanupLogs(); }, '过期日志已清理')}
              disabled={busyAction === 'cleanup'}
            >
              <Trash2 size={14} /> {busyAction === 'cleanup' ? '清理中...' : '清理日志'}
            </button>
          </div>
        </section>

        <section className="card dashboard-panel">
          <div className="card-header-inline">
            <div>
              <h4>Recent Requests</h4>
              <p className="section-subtle">快速查看最近流量并跳转到请求流页。</p>
            </div>
          </div>
          <div className="request-list-compact">
            {overview.recent_requests.length > 0 ? overview.recent_requests.map((request) => (
              <button key={request.id} className="request-row-compact" onClick={() => navigate('/requests')}>
                <div>
                  <div className="cell-main">{request.downstream_path}</div>
                  <div className="cell-sub">{request.model || '-'} · {new Date(request.created_at).toLocaleString()}</div>
                </div>
                <span className={`status-chip ${request.status === 'success' ? 'ok' : 'err'}`}>
                  {request.upstream_status || request.status}
                </span>
              </button>
            )) : (
              <div className="chart-empty">暂无最近请求</div>
            )}
          </div>
          <button className="link-button-inline" onClick={() => navigate('/requests')}>
            查看请求流 <ArrowRight size={14} />
          </button>
        </section>
      </div>

      <div className="dashboard-chart-grid dashboard-chart-grid-spaced">
        <div className="card">
          <h4 style={{ marginBottom: 12 }}>成功率趋势</h4>
          <ResponsiveContainer width="100%" height={260}>
            <LineChart data={chartData.success_rate_series}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
              <XAxis dataKey="time" tick={{ fontSize: 11 }} tickFormatter={(t) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} />
              <YAxis domain={[0, 100]} tick={{ fontSize: 11 }} />
              <Tooltip />
              <Line type="monotone" dataKey="rate" stroke="var(--primary)" strokeWidth={2} dot={false} />
            </LineChart>
          </ResponsiveContainer>
        </div>

        <div className="card">
          <h4 style={{ marginBottom: 12 }}>Token 趋势</h4>
          <ResponsiveContainer width="100%" height={260}>
            <BarChart data={chartData.token_series}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
              <XAxis dataKey="time" tick={{ fontSize: 11 }} tickFormatter={(t) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} />
              <YAxis tick={{ fontSize: 11 }} />
              <Tooltip />
              <Legend />
              <Bar dataKey="prompt" fill="#cc785c" name="Prompt" stackId="a" />
              <Bar dataKey="completion" fill="#5db8a6" name="Completion" stackId="a" />
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div className="card">
          <h4 style={{ marginBottom: 12 }}>模型分布</h4>
          <ResponsiveContainer width="100%" height={260}>
            <PieChart>
              <Pie data={chartData.model_distribution} dataKey="count" nameKey="model" cx="50%" cy="50%" outerRadius={90} label>
                {chartData.model_distribution.map((_, index) => <Cell key={index} fill={COLORS[index % COLORS.length]} />)}
              </Pie>
              <Tooltip />
            </PieChart>
          </ResponsiveContainer>
        </div>

        <div className="card">
          <h4 style={{ marginBottom: 12 }}>Top 模型</h4>
          <ResponsiveContainer width="100%" height={260}>
            <BarChart data={chartData.model_distribution.slice(0, 6)} layout="vertical">
              <XAxis type="number" tick={{ fontSize: 11 }} />
              <YAxis type="category" dataKey="model" tick={{ fontSize: 11 }} width={120} />
              <Tooltip />
              <Bar dataKey="count" fill="var(--primary)" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </div>
    </div>
  );
}

function requestLatency(item: RequestLog) {
  if (item.upstream_ms > 0) return item.upstream_ms;
  if (item.downstream_ms > 0) return item.downstream_ms;
  if (item.ttft_ms > 0) return item.ttft_ms;
  return 0;
}
