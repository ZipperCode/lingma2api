import { useEffect, useMemo, useState } from 'react';
import { createPolicy, deletePolicy, getPolicies, testPolicy, updatePolicy } from '../api/client';
import { PolicyRuleEditor } from '../components/PolicyRuleEditor';
import type { PolicyRule, PolicyTestInput, PolicyTestResult } from '../types';

const emptyTest: PolicyTestInput = {
  protocol: 'openai',
  requested_model: 'gpt-4',
  stream: false,
  has_tools: false,
  has_reasoning: false,
  session_present: false,
};

export function Policies() {
  const [policies, setPolicies] = useState<PolicyRule[]>([]);
  const [editing, setEditing] = useState<PolicyRule | null>(null);
  const [showNew, setShowNew] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [testInput, setTestInput] = useState<PolicyTestInput>(emptyTest);
  const [testResult, setTestResult] = useState<PolicyTestResult | null>(null);

  const enabledCount = useMemo(() => policies.filter(p => p.enabled).length, [policies]);

  const load = async () => {
    setLoading(true);
    setError('');
    try {
      setPolicies(await getPolicies());
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载策略失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const handleSave = async (data: Partial<PolicyRule>) => {
    if (editing) {
      await updatePolicy(editing.id, data);
    } else {
      await createPolicy(data);
    }
    setEditing(null);
    setShowNew(false);
    load();
  };

  const handleDelete = async (id: number) => {
    if (!confirm('确认删除该策略？')) return;
    await deletePolicy(id);
    load();
  };

  const handleTest = async () => {
    setTestResult(await testPolicy(testInput));
  };

  return (
    <div>
      <div className="page-header">
        <div>
          <h2>策略引擎</h2>
          <p style={{ color: 'var(--text-secondary)', marginTop: 6 }}>
            基于 canonical request 的执行策略，优先于兼容模型映射。
          </p>
        </div>
        <button className="btn btn-primary" onClick={() => setShowNew(true)}>➕ 新增策略</button>
      </div>

      <div className="policy-summary-grid">
        <div className="card policy-summary-card">
          <span>策略总数</span>
          <strong>{policies.length}</strong>
        </div>
        <div className="card policy-summary-card">
          <span>启用中</span>
          <strong>{enabledCount}</strong>
        </div>
        <div className="card policy-summary-card">
          <span>评估顺序</span>
          <strong>Priority ↑</strong>
        </div>
      </div>

      {error && <div className="card" style={{ color: 'var(--error)', marginBottom: 16 }}>{error}</div>}

      <div className="card" style={{ marginBottom: 16 }}>
        <h4 style={{ marginBottom: 12 }}>策略规则</h4>
        {loading ? <div>加载中...</div> : (
          <table>
            <thead>
              <tr><th>优先级</th><th>名称</th><th>匹配</th><th>动作</th><th>状态</th><th>操作</th></tr>
            </thead>
            <tbody>
              {policies.map(policy => (
                <tr key={policy.id}>
                  <td>{policy.priority}</td>
                  <td>
                    <strong>{policy.name}</strong>
                    {policy.source && <div style={{ color: 'var(--text-secondary)', fontSize: 12 }}>{policy.source}</div>}
                  </td>
                  <td><PolicyChips value={policy.match} /></td>
                  <td><PolicyChips value={policy.actions} /></td>
                  <td>{policy.enabled ? '✅' : '❌'}</td>
                  <td>
                    <button className="btn" onClick={() => setEditing(policy)} style={{ marginRight: 4 }}>✏️</button>
                    <button className="btn btn-danger" onClick={() => handleDelete(policy.id)}>🗑</button>
                  </td>
                </tr>
              ))}
              {policies.length === 0 && (
                <tr><td colSpan={6} style={{ color: 'var(--text-secondary)' }}>暂无策略，创建一条模型重写或工具控制规则开始。</td></tr>
              )}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <h4 style={{ marginBottom: 12 }}>策略测试</h4>
        <div className="form-grid four-cols">
          <div className="form-group">
            <label>协议</label>
            <select className="input" value={testInput.protocol} onChange={e => setTestInput({ ...testInput, protocol: e.target.value })}>
              <option value="openai">OpenAI</option>
              <option value="anthropic">Anthropic</option>
            </select>
          </div>
          <div className="form-group">
            <label>请求模型</label>
            <input className="input" value={testInput.requested_model} onChange={e => setTestInput({ ...testInput, requested_model: e.target.value })} />
          </div>
          <CheckField label="流式" checked={testInput.stream} onChange={stream => setTestInput({ ...testInput, stream })} />
          <CheckField label="工具" checked={testInput.has_tools} onChange={has_tools => setTestInput({ ...testInput, has_tools })} />
          <CheckField label="推理" checked={testInput.has_reasoning} onChange={has_reasoning => setTestInput({ ...testInput, has_reasoning })} />
          <CheckField label="会话" checked={testInput.session_present} onChange={session_present => setTestInput({ ...testInput, session_present })} />
          <div className="form-group">
            <label>客户端</label>
            <input className="input" value={testInput.client_name || ''} onChange={e => setTestInput({ ...testInput, client_name: e.target.value })} />
          </div>
          <div className="form-group">
            <label>入口标签</label>
            <input className="input" value={testInput.ingress_tag || ''} onChange={e => setTestInput({ ...testInput, ingress_tag: e.target.value })} />
          </div>
        </div>
        <button className="btn" onClick={handleTest}>测试策略</button>
        {testResult && (
          <div className="policy-test-result">
            <div>{testResult.matched ? '✅ 命中策略' : '❌ 未命中策略'}</div>
            <PolicyChips value={testResult.effective_actions} />
            {testResult.matched_rules?.length > 0 && (
              <div style={{ marginTop: 8, color: 'var(--text-secondary)', fontSize: 13 }}>
                命中：{testResult.matched_rules.map(rule => `${rule.name}#${rule.id}`).join(', ')}
              </div>
            )}
          </div>
        )}
      </div>

      {(showNew || editing) && (
        <PolicyRuleEditor
          policy={editing || undefined}
          onSave={handleSave}
          onClose={() => { setEditing(null); setShowNew(false); }}
        />
      )}
    </div>
  );
}

function PolicyChips({ value }: { value: object }) {
  const entries = Object.entries(value || {}).filter(([, item]) => item !== undefined && item !== '' && !(Array.isArray(item) && item.length === 0));
  if (entries.length === 0) return <span style={{ color: 'var(--text-secondary)' }}>任意</span>;
  return <>{entries.map(([key, item]) => <span className="tag" key={key}>{key}: {Array.isArray(item) ? item.join(',') : String(item)}</span>)}</>;
}

function CheckField({ label, checked, onChange }: { label: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <div className="form-group">
      <label>{label}</label>
      <label className="checkbox-pill">
        <input type="checkbox" checked={checked} onChange={e => onChange(e.target.checked)} />
        {checked ? '是' : '否'}
      </label>
    </div>
  );
}
