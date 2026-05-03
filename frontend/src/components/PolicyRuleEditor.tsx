import { useState } from 'react';
import type { PolicyRule } from '../types';

interface Props {
  policy?: PolicyRule;
  onSave: (policy: Partial<PolicyRule>) => void;
  onClose: () => void;
}

const boolToSelect = (value?: boolean) => value === undefined ? '' : value ? 'true' : 'false';
const selectToBool = (value: string) => value === '' ? undefined : value === 'true';

export function PolicyRuleEditor({ policy, onSave, onClose }: Props) {
  const [name, setName] = useState(policy?.name || '');
  const [priority, setPriority] = useState(policy?.priority ?? 0);
  const [enabled, setEnabled] = useState(policy?.enabled ?? true);
  const [protocol, setProtocol] = useState(policy?.match?.protocol || '');
  const [requestedModel, setRequestedModel] = useState(policy?.match?.requested_model || '');
  const [stream, setStream] = useState(boolToSelect(policy?.match?.stream));
  const [hasTools, setHasTools] = useState(boolToSelect(policy?.match?.has_tools));
  const [hasReasoning, setHasReasoning] = useState(boolToSelect(policy?.match?.has_reasoning));
  const [sessionPresent, setSessionPresent] = useState(boolToSelect(policy?.match?.session_present));
  const [clientName, setClientName] = useState(policy?.match?.client_name || '');
  const [ingressTag, setIngressTag] = useState(policy?.match?.ingress_tag || '');
  const [rewriteModel, setRewriteModel] = useState(policy?.actions?.rewrite_model || '');
  const [setReasoning, setSetReasoning] = useState(boolToSelect(policy?.actions?.set_reasoning));
  const [allowTools, setAllowTools] = useState(boolToSelect(policy?.actions?.allow_tools));
  const [addTags, setAddTags] = useState(policy?.actions?.add_tags?.join(', ') || '');
  const [error, setError] = useState('');

  const handleSave = () => {
    if (!name.trim()) {
      setError('策略名称必填');
      return;
    }
    if (requestedModel.trim()) {
      try { new RegExp(requestedModel.trim()); } catch {
        setError('模型匹配正则无效');
        return;
      }
    }

    const match = {
      ...(protocol ? { protocol } : {}),
      ...(requestedModel.trim() ? { requested_model: requestedModel.trim() } : {}),
      ...(selectToBool(stream) !== undefined ? { stream: selectToBool(stream) } : {}),
      ...(selectToBool(hasTools) !== undefined ? { has_tools: selectToBool(hasTools) } : {}),
      ...(selectToBool(hasReasoning) !== undefined ? { has_reasoning: selectToBool(hasReasoning) } : {}),
      ...(selectToBool(sessionPresent) !== undefined ? { session_present: selectToBool(sessionPresent) } : {}),
      ...(clientName.trim() ? { client_name: clientName.trim() } : {}),
      ...(ingressTag.trim() ? { ingress_tag: ingressTag.trim() } : {}),
    };
    const actions = {
      ...(rewriteModel.trim() ? { rewrite_model: rewriteModel.trim() } : {}),
      ...(selectToBool(setReasoning) !== undefined ? { set_reasoning: selectToBool(setReasoning) } : {}),
      ...(selectToBool(allowTools) !== undefined ? { allow_tools: selectToBool(allowTools) } : {}),
      ...(addTags.trim() ? { add_tags: addTags.split(',').map(t => t.trim()).filter(Boolean) } : {}),
    };
    if (Object.keys(actions).length === 0) {
      setError('至少配置一个动作');
      return;
    }
    onSave({ name: name.trim(), priority, enabled, match, actions });
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()}>
        <h3>{policy ? '编辑策略' : '新增策略'}</h3>
        {error && <div style={{ color: 'var(--error)', marginBottom: 12 }}>{error}</div>}
        <div className="form-grid two-cols">
          <div className="form-group">
            <label>策略名称</label>
            <input className="input" value={name} onChange={e => setName(e.target.value)} />
          </div>
          <div className="form-group">
            <label>优先级（越小越高）</label>
            <input className="input" type="number" value={priority} onChange={e => setPriority(Number(e.target.value))} />
          </div>
        </div>
        <div className="form-section-title">匹配 canonical 属性</div>
        <div className="form-grid two-cols">
          <div className="form-group">
            <label>协议</label>
            <select className="input" value={protocol} onChange={e => setProtocol(e.target.value)}>
              <option value="">任意</option>
              <option value="openai">OpenAI</option>
              <option value="anthropic">Anthropic</option>
            </select>
          </div>
          <div className="form-group">
            <label>请求模型正则</label>
            <input className="input" value={requestedModel} onChange={e => setRequestedModel(e.target.value)} placeholder="^gpt-4" />
          </div>
          <SelectBool label="流式" value={stream} onChange={setStream} />
          <SelectBool label="包含工具" value={hasTools} onChange={setHasTools} />
          <SelectBool label="包含推理" value={hasReasoning} onChange={setHasReasoning} />
          <SelectBool label="有会话" value={sessionPresent} onChange={setSessionPresent} />
          <div className="form-group">
            <label>客户端名称</label>
            <input className="input" value={clientName} onChange={e => setClientName(e.target.value)} />
          </div>
          <div className="form-group">
            <label>入口标签</label>
            <input className="input" value={ingressTag} onChange={e => setIngressTag(e.target.value)} />
          </div>
        </div>
        <div className="form-section-title">动作</div>
        <div className="form-grid two-cols">
          <div className="form-group">
            <label>重写模型</label>
            <input className="input" value={rewriteModel} onChange={e => setRewriteModel(e.target.value)} placeholder="目标 Lingma 模型" />
          </div>
          <SelectBool label="设置推理" value={setReasoning} onChange={setSetReasoning} />
          <SelectBool label="允许工具" value={allowTools} onChange={setAllowTools} />
          <div className="form-group">
            <label>附加标签（逗号分隔）</label>
            <input className="input" value={addTags} onChange={e => setAddTags(e.target.value)} placeholder="vip, audit" />
          </div>
        </div>
        <div className="form-group">
          <label>
            <input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} style={{ marginRight: 8 }} />
            启用策略
          </label>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button className="btn btn-primary" onClick={handleSave}>保存</button>
          <button className="btn" onClick={onClose}>取消</button>
        </div>
      </div>
    </div>
  );
}

function SelectBool({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <div className="form-group">
      <label>{label}</label>
      <select className="input" value={value} onChange={e => onChange(e.target.value)}>
        <option value="">任意/不设置</option>
        <option value="true">是</option>
        <option value="false">否</option>
      </select>
    </div>
  );
}
