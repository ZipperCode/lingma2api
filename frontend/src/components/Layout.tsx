import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { LayoutDashboard, ScrollText, User, Shield, Settings, Sun, Moon, Circle, Boxes } from 'lucide-react';
import type { Theme } from '../types';

interface Props {
  theme: Theme;
  onToggleTheme: () => void;
}

export function Layout({ theme, onToggleTheme }: Props) {
  const location = useLocation();
  const meta = pageMeta[location.pathname] ?? pageMeta.default;

  return (
    <div className="app-layout">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <div className="sidebar-brand-mark">L2A</div>
          <div className="sidebar-brand-copy">
            <div className="sidebar-logo">lingma2api</div>
            <div className="sidebar-tagline">OpenAI / Anthropic bridge console</div>
          </div>
        </div>
        <nav className="sidebar-nav">
          <NavLink to="/dashboard" className={({ isActive }) => isActive ? 'active' : ''}>
            <LayoutDashboard size={18} /> 仪表盘
          </NavLink>
          <NavLink to="/requests" className={({ isActive }) => isActive ? 'active' : ''}>
            <ScrollText size={18} /> 请求流
          </NavLink>
          <NavLink to="/models" className={({ isActive }) => isActive ? 'active' : ''}>
            <Boxes size={18} /> 模型
          </NavLink>
          <NavLink to="/account" className={({ isActive }) => isActive ? 'active' : ''}>
            <User size={18} /> 账号管理
          </NavLink>
          <NavLink to="/policies" className={({ isActive }) => isActive ? 'active' : ''}>
            <Shield size={18} /> 策略引擎
          </NavLink>
          <NavLink to="/settings" className={({ isActive }) => isActive ? 'active' : ''}>
            <Settings size={18} /> 设置
          </NavLink>
        </nav>
        <div className="sidebar-status">
          <Circle size={10} fill="currentColor" style={{ color: 'var(--success)', marginRight: 6, verticalAlign: 'middle' }} />
          已连接
        </div>
      </aside>
      <div className="main-area">
        <div className="top-bar">
          <div className="top-bar-inner">
            <div className="top-bar-copy">
              <span className="top-bar-kicker">{meta.kicker}</span>
              <div>
                <h1>{meta.title}</h1>
                <p>{meta.description}</p>
              </div>
            </div>
            <div className="top-bar-actions">
              <button className="btn" onClick={onToggleTheme}>
                {theme === 'light' ? <Moon size={16} /> : <Sun size={16} />}
                <span>{theme === 'light' ? '深色' : '浅色'}</span>
              </button>
              <NavLink to="/settings" className="btn">
                <Settings size={16} /> 设置
              </NavLink>
            </div>
          </div>
        </div>
        <div className="content">
          <div className="content-inner">
            <Outlet />
          </div>
        </div>
        <div className="bottom-bar">
          <div className="bottom-bar-inner">
            <span>lingma2api v1.0.0</span>
            <span>Remote-first console for Lingma upstream contracts</span>
          </div>
        </div>
      </div>
    </div>
  );
}

const pageMeta: Record<string, { title: string; description: string; kicker: string }> = {
  '/dashboard': {
    title: '仪表盘',
    description: '集中查看代理状态、成功率、TTFT 与 token 趋势。',
    kicker: 'Overview',
  },
  '/logs': {
    title: '历史日志',
    description: '按时间线检查历史请求、回放与 canonical 记录。',
    kicker: 'Traffic',
  },
  '/requests': {
    title: '请求流',
    description: '查看最近请求、筛选状态并展开上下游内容。',
    kicker: 'Requests',
  },
  '/models': {
    title: '模型',
    description: '检查当前可用模型、缓存状态与快速复制模型 ID。',
    kicker: 'Models',
  },
  '/account': {
    title: '账号管理',
    description: '处理授权文件、刷新凭据与账户状态检查。',
    kicker: 'Credentials',
  },
  '/policies': {
    title: '策略引擎',
    description: '基于 canonical 属性做匹配、重写与测试。',
    kicker: 'Policy',
  },
  '/settings': {
    title: '设置',
    description: '管理存储、刷新频率、超时与控制台安全项。',
    kicker: 'System',
  },
  default: {
    title: '控制台',
    description: '统一管理 Lingma 远端代理运行态。',
    kicker: 'Console',
  },
};
