import { NavLink, Outlet } from 'react-router-dom';
import { LayoutDashboard, ScrollText, User, Shield, Settings, Sun, Moon, Circle, Boxes } from 'lucide-react';
import type { Theme } from '../types';

interface Props {
  theme: Theme;
  onToggleTheme: () => void;
}

export function Layout({ theme, onToggleTheme }: Props) {
  return (
    <div className="app-layout">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <div className="sidebar-brand-mark">L2A</div>
          <div className="sidebar-brand-copy">
            <div className="sidebar-logo">lingma2api</div>
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
        </nav>
        <div className="sidebar-footer">
          <button className="sidebar-footer-btn" onClick={onToggleTheme} title="切换主题">
            {theme === 'light' ? <Moon size={15} /> : <Sun size={15} />}
          </button>
          <NavLink to="/settings" className="sidebar-footer-btn" title="设置">
            <Settings size={15} />
          </NavLink>
          <span className="sidebar-footer-status">
            <Circle size={8} fill="currentColor" style={{ color: 'var(--success)' }} />
            已连接
          </span>
        </div>
      </aside>
      <div className="main-area">
        <div className="content">
          <div className="content-inner">
            <Outlet />
          </div>
        </div>
        <div className="bottom-bar">
          <div className="bottom-bar-inner">
            <span>lingma2api v1.0.0</span>
          </div>
        </div>
      </div>
    </div>
  );
}
