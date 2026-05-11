import { useCallback, useEffect, useMemo, useState } from 'react';
import { Copy, RefreshCw, Search } from 'lucide-react';
import { getAdminModels, refreshAdminModels } from '../api/client';
import { EmptyState } from '../components/EmptyState';
import { Skeleton } from '../components/Skeleton';
import type { AdminModelsResponse } from '../types';

export function Models() {
  const [data, setData] = useState<AdminModelsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [query, setQuery] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setData(await getAdminModels());
    } catch {
      setData({ items: [], status: { fetched_at: '', cached: false, count: 0 } });
    }
    setLoading(false);
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      setData(await refreshAdminModels());
    } catch {
      // ignore
    }
    setRefreshing(false);
  };

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const items = data?.items ?? [];
    if (!q) return items;
    return items.filter((item) => `${item.id} ${item.owned_by}`.toLowerCase().includes(q));
  }, [data, query]);

  return (
    <div>
      <div className="page-header">
        <h2>模型</h2>
        <div className="page-actions page-actions-wrap">
          <label className="search-field models-search-field">
            <Search size={15} />
            <input
              className="input"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索模型 ID"
            />
          </label>
          <button className="btn btn-primary" onClick={handleRefresh} disabled={refreshing}>
            <RefreshCw size={16} /> {refreshing ? '刷新中...' : '刷新模型'}
          </button>
        </div>
      </div>

      {loading ? (
        <div className="models-grid">
          <Skeleton variant="rect" style={{ height: 120 }} />
          <Skeleton variant="rect" style={{ height: 120 }} />
          <Skeleton variant="rect" style={{ height: 120 }} />
        </div>
      ) : (
        <>
          <div className="models-metrics">
            <div className="card models-metric-card">
              <span>缓存状态</span>
              <strong>{data?.status.cached ? 'Ready' : 'Empty'}</strong>
            </div>
            <div className="card models-metric-card">
              <span>模型总数</span>
              <strong>{data?.status.count ?? 0}</strong>
            </div>
            <div className="card models-metric-card">
              <span>最近刷新</span>
              <strong>{data?.status.fetched_at ? new Date(data.status.fetched_at).toLocaleString() : '未刷新'}</strong>
            </div>
          </div>

          <div className="card">
            <div className="card-header-inline">
              <div>
                <h4>可用模型</h4>
                <p className="section-subtle">优先展示来自 Lingma 上游的真实模型与别名映射。</p>
              </div>
              <div className="page-meta-inline">{filtered.length} models</div>
            </div>

            {filtered.length === 0 ? (
              <EmptyState
                icon={Copy}
                title="暂无模型"
                description="请先刷新模型缓存，或检查凭据与上游连接状态。"
              />
            ) : (
              <div className="models-list-grid">
                {filtered.map((item) => (
                  <button
                    key={item.id}
                    className="model-tile"
                    onClick={() => navigator.clipboard.writeText(item.id)}
                    title={`复制模型 ID：${item.id}`}
                  >
                    <div className="model-tile-top">
                      <span className={`status-chip ${item.owned_by === 'lingma' ? 'ok' : 'warn'}`}>
                        {item.owned_by === 'lingma' ? 'Upstream' : 'Alias'}
                      </span>
                      <Copy size={14} />
                    </div>
                    <div className="model-tile-id">{item.id}</div>
                    <div className="model-tile-meta">
                      <span>{specForModel(item.id).context}</span>
                      <span>{specForModel(item.id).capability}</span>
                    </div>
                  </button>
                ))}
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}

function specForModel(id: string) {
  const key = id.toLowerCase();
  if (key.includes('kimi') || key.includes('kmodel')) {
    return { context: '256K', capability: '文本 / 工具 / 多轮' };
  }
  if (key.includes('minimax') || key.includes('mmodel')) {
    return { context: '200K', capability: 'Agent / Tool Use' };
  }
  if (key.includes('coder')) {
    return { context: '1M', capability: 'Code / Thinking / Tools' };
  }
  if (key.includes('thinking')) {
    return { context: '256K', capability: 'Reasoning / Tools' };
  }
  if (key.includes('qwen')) {
    return { context: '256K+', capability: 'General / Structured' };
  }
  if (key === 'auto') {
    return { context: 'Auto', capability: 'Alias / Auto route' };
  }
  return { context: 'N/A', capability: 'General' };
}
